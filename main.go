package main

import (
	"bytes"
	"errors"
	"flag"
	"io/ioutil"
	"strings"
	"text/template"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/ecr"
	log "github.com/sirupsen/logrus"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

const appVersion = "0.1.0"

const cfgTemplate = `{
  "auths": {
	 "{{ .registry }}": {
	   "auth": "{{ .token }}"
	 }
  }
}`

func init() {
	log.SetFormatter(&log.JSONFormatter{})
}

func main() {
	log.Infof("appVersion: %s", appVersion)

	// Flags
	region := flag.String("region", "", "The aws region")
	interval := flag.Int("interval", 1200, "Refresh interval in seconds")
	profile := flag.String("profile", "", "The AWS Account profile")
	secretName := flag.String("secretName", "ecr-auth-cfg", "The name of the secret")
	stripScheme := flag.Bool("stripScheme", false, "Remove the scheme from the registry URL")
	flag.Parse()
	log.Infof("Flags: region=%s, interval=%d, profile=%s, secretName=%s",
		*region, *interval, *profile, *secretName)

	// Validate important variables
	if *region == "" {
		panic("Region not specified")
	}

	// Start a new aws client session
	sess := session.Must(session.NewSession(&aws.Config{
		Region: region,
	}))
	svc := ecr.New(sess)

	for {
		// Get the ECR authorization token from AWS
		tokenInput := &ecr.GetAuthorizationTokenInput{}
		if *profile != "" {
			tokenInput.RegistryIds = []*string{profile}
		}
		token, err := svc.GetAuthorizationToken(tokenInput)
		if err != nil {
			panic(err.Error())
		}

		// Create the docker config.json in buffer
		dockerCfg, err := createDockerCfg(token, *stripScheme)

		// Get current namespace
		namespace, err := ioutil.ReadFile("/var/run/secrets/kubernetes.io/serviceaccount/namespace")
		if err != nil {
			panic(err.Error())
		}

		// Create the docker config.json as a kubernetes secret
		kconfig, err := rest.InClusterConfig()
		if err != nil {
			panic(err.Error())
		}
		clientSet, err := kubernetes.NewForConfig(kconfig)
		if err != nil {
			panic(err.Error())
		}
		kclient := &kubernetesAPI{client: clientSet}
		err = kclient.applyDockerCfgSecret(dockerCfg, *secretName, string(namespace))
		if err != nil {
			panic(err.Error())
		}

		// sleep interval
		time.Sleep(time.Duration(*interval) * time.Second)
	}

}

func createDockerCfg(ecrToken *ecr.GetAuthorizationTokenOutput, stripScheme bool) ([]byte, error) {
	if len(ecrToken.AuthorizationData) < 1 {
		return nil, errors.New("authorization data should have at least 1 auth data")
	}
	cfgData := map[string]string{}
	log.Infoln(*ecrToken.AuthorizationData[0].ProxyEndpoint)
	if stripScheme {
		splits := strings.Split(*ecrToken.AuthorizationData[0].ProxyEndpoint, "://")
		cfgData["registry"] = splits[1]
	} else {
		cfgData["registry"] = *ecrToken.AuthorizationData[0].ProxyEndpoint
	}
	log.Infoln(cfgData["registry"])
	cfgData["token"] = *ecrToken.AuthorizationData[0].AuthorizationToken

	// Put the config template output in a buffer
	t := template.Must(template.New("").Parse(cfgTemplate))
	cfgBuffer := bytes.NewBufferString("")

	if err := t.Execute(cfgBuffer, cfgData); err != nil {
		return nil, err
	}

	cfgInByte, err := ioutil.ReadAll(cfgBuffer)
	if err != nil {
		return nil, err
	}

	return cfgInByte, nil
}

type kubernetesAPI struct {
	client kubernetes.Interface
}

func (k *kubernetesAPI) applyDockerCfgSecret(cfg []byte, secretName, namespace string) error {
	secret := &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name: secretName,
		},
		Data: map[string][]byte{
			"config.json": cfg,
		},
	}

	log.Infoln("creating kubernetes secret")
	secretClient := k.client.CoreV1().Secrets(namespace)
	result, err := secretClient.Update(secret)
	actionTaken := "updated"
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			result, err = secretClient.Create(secret)
			if err != nil {
				return err
			}
			actionTaken = "created"
		} else {
			return err
		}
	}

	log.Infof("%s kubernetes secret: %s", actionTaken, result.GetObjectMeta().GetName())
	return nil
}
