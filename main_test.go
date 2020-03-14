package main

import (
	"testing"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/ecr"
	v1 "k8s.io/api/core/v1"
	testclient "k8s.io/client-go/kubernetes/fake"
)

func TestCreateDockerCfg(t *testing.T) {
	tt := []struct {
		tokenOutput *ecr.GetAuthorizationTokenOutput
		success     bool
		name        string
	}{
		{
			tokenOutput: &ecr.GetAuthorizationTokenOutput{
				AuthorizationData: []*ecr.AuthorizationData{
					&ecr.AuthorizationData{
						ProxyEndpoint:      aws.String("xxx"),
						AuthorizationToken: aws.String("xxx"),
					},
				}},
			success: true,
			name:    "with valid ecr token output",
		},
		{
			tokenOutput: &ecr.GetAuthorizationTokenOutput{},
			success:     false,
			name:        "empty ecr token output",
		},
	}

	for _, tc := range tt {
		t.Run(tc.name, func(t *testing.T) {
			_, err := createDockerCfg(tc.tokenOutput)
			if err != nil {
				if tc.success {
					t.Fatal(err)
				}
			}
		})
	}
}

func TestApplyDockerCfgSecret(t *testing.T) {
	kclient := &kubernetesAPI{client: testclient.NewSimpleClientset()}
	err := kclient.applyDockerCfgSecret(
		[]byte("test"), "docker-secret", "test-namespace", v1.SecretTypeOpaque)
	if err != nil {
		t.Fatal(err)
	}
}
