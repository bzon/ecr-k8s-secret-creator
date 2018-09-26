package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"strings"
	"text/template"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/ecr"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

const cfgTemplate = `{
  "auths": {
	 "{{ .registry }}": {
	   "auth": "{{ .token }}"
	 }
  }
}`

func main() {
	// Parse flags
	region := flag.String("region", "", "The aws region")
	interval := flag.Int("interval", 1200, "Refresh interval in seconds")
	profile := flag.String("profile", "", "The AWS Account profile")
	secretName := flag.String("secretName", "ecr-auth-cfg", "The name of the secret")
	flag.Parse()

	// Validate variables
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
		if len(token.AuthorizationData) < 1 {
			panic("token.AuthorizationData slice length is zero. " +
				"No authorization token found.")
		}

		// Create the docker config.json in buffer
		dockerCfg, err := createDockerCfg(token)

		// Create the docker config.json as a kubernetes secret
		applyDockerCfgSecret(dockerCfg, *secretName)

		time.Sleep(time.Duration(*interval) * time.Second)
	}

}

func createDockerCfg(ecrToken *ecr.GetAuthorizationTokenOutput) ([]byte, error) {
	cfgData := map[string]string{}
	cfgData["registry"] = *ecrToken.AuthorizationData[0].ProxyEndpoint
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

func applyDockerCfgSecret(cfg []byte, secretName string) {
	secret := &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name: secretName,
		},
		Data: map[string][]byte{
			"config.json": cfg,
		},
	}

	// Print secret manifest for troubleshooting :)
	cfgInByte, err := json.MarshalIndent(secret, "", " ")
	if err != nil {
		panic(err.Error())
	}
	fmt.Println("Creating secret..")
	fmt.Println(string(cfgInByte))

	config, err := rest.InClusterConfig()
	if err != nil {
		panic(err.Error())
	}

	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		panic(err.Error())
	}

	// Get service acccount namespace
	namespace, err := ioutil.ReadFile("/var/run/secrets/kubernetes.io/serviceaccount/namespace")
	if err != nil {
		panic(err.Error())
	}

	// Create the secret
	secretClient := clientset.CoreV1().Secrets(string(namespace))
	result, err := secretClient.Update(secret)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			result, err = secretClient.Create(secret)
			if err != nil {
				panic(err.Error())
			}
		} else {
			panic(err.Error())
		}
	}

	fmt.Printf("Created/Updated a secret %q.\n", result.GetObjectMeta().GetName())
}
