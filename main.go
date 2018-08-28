package main

import (
	"encoding/base64"
	"flag"
	"fmt"
	"os"
	"strings"
	"text/template"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/ecr"

	v1 "k8s.io/api/core/v1"
)

func main() {

	// Parse flags
	region := flag.String("region", "", "The aws region")
	profile := flag.String("profile", "", "The aws account id")
	flag.Parse()

	// Validate variables
	if *region == "" {
		panic("Region not specified")
	}
	if *profile == "" {
		panic("Profile not specified")
	}

	// Start a new aws client session
	sess := session.Must(session.NewSession(&aws.Config{
		Region: region,
	}))
	svc := ecr.New(sess)

	// Set the registry id using profile
	tokenInput := &ecr.GetAuthorizationTokenInput{
		RegistryIds: []*string{profile},
	}
	token, err := svc.GetAuthorizationToken(tokenInput)
	if err != nil {
		panic(err)
	}
	createDockerCfg(token)
}

//
// {
//   "auths": {
//     "${ECR_REGISTRY}": {
//       "auth": "${BASE64_AUTH}"
//     }
//   }
// }
const cfgTemplate = `{
	"auths": {
		"{{ .registry }}": {
			"auth": "{{ .token }}"
		}
	}
}`

const loginCmdTemplate = `docker login -u AWS -p {{ .loginPassword }} {{ .registry }}`

func createDockerCfg(ecrToken *ecr.GetAuthorizationTokenOutput) {
	cfgData := map[string]string{}

	cfgData["registry"] = *ecrToken.AuthorizationData[0].ProxyEndpoint
	cfgData["token"] = *ecrToken.AuthorizationData[0].AuthorizationToken

	// Decode authentication to base64
	encodedToken, err :=
		base64.StdEncoding.DecodeString(*ecrToken.AuthorizationData[0].AuthorizationToken)
	if err != nil {
		panic(err)
	}
	// Remove the AWS: part of the token
	cfgData["loginPassword"] = strings.SplitN(string(encodedToken), ":", 2)[1]

	fmt.Println("############################################")
	fmt.Println("This is the docker config.json file\n")
	t := template.Must(template.New("").Parse(cfgTemplate))
	if err := t.Execute(os.Stdout, cfgData); err != nil {
		panic(err)
	}
	fmt.Println("############################################")
	fmt.Println("############################################")
	fmt.Println("This is the docker login command\n")
	t2 := template.Must(template.New("").Parse(loginCmdTemplate))
	if err := t2.Execute(os.Stdout, cfgData); err != nil {
		panic(err)
	}

}

func createKubernetesSecret(cfg []byte) error {
	secret := &v1.Secret{}
	secret.Data["config.json"] = []byte(cfg)
	return nil
}
