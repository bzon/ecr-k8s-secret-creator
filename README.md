![Docker Pulls](https://img.shields.io/docker/pulls/bzon/ecr-k8s-secret-creator.svg)[![Go Report Card](https://goreportcard.com/badge/github.com/bzon/ecr-k8s-secret-creator)](https://goreportcard.com/report/github.com/bzon/ecr-k8s-secret-creator)

# ECR K8S Secret Creator

This application creates a docker config.json (as a Kubernetes secret) that can authenticate docker clients to Amazon ECR. It is using the [ECR GetAuthorizationToken API](https://docs.aws.amazon.com/AmazonECR/latest/APIReference/API_GetAuthorizationToken.html) to fetch the token from an Amazon Region.

A docker config.json file looks like this, which may be found in `$HOME/.docker/config.json` after using `docker login`.

```json
{
  "auths": {
	 "https://${AWS_PROFILE}.dkr.ecr.us-east-1.amazonaws.com": {
	   "auth": "....."
	 }
  }
}
```

This application is like a running cron job that does `aws ecr get-login`, creates a docker config.json file, then create Kubernetes secret out of it.

## How does it work?

* Deploy this application as a Pod in the namespace to create the Secret.

* This Pod authenticates to AWS to get a new ECR docker login token according to the `-region` flag.

* It then creates a config.json according to the values retrieved from AWS.

    ```go
    # config.json template
    const cfgTemplate = `{
      "auths": {
       "{{ .registry }}": {
         "auth": "{{ .token }}"
       }
      }
    }`
    ```

* It then creates or updates the Secret according to the `-secretName` flag in the current namespace.

    ```yaml
    apiVersion: v1
    type: Secret
    metadata:
      name: xxxx # -secretName
      namespace: xxxx # current pod namespace
    data:
      config.json: xxxxx # base64 encoded
    ```

* It repeats the process according to the specified `-interval` flag. Refreshing the config.json file content in the Secret over the time specified.

* You can now use this kubernetes secret and __mount__ it to any pod that has a docker client that authenticates to ECR.


## Why did I create this?

We are using [Weave Flux](https://github.com/weaveworks/flux) to operate our __GitOps__ deployment process. The Weave Flux operator currently does not support authentication to ECR ([issue #539](https://github.com/weaveworks/flux/issues/539)). As a workaround, we can use the `--docker-config` flag and mount a custom `config.json` in the flux Pod ([issue #1065](https://github.com/weaveworks/flux/pull/1065)).

The problem is ECR token expires every 12 hours, and we need to find a way to ensure that the config.json authentication token is rotated in an automated and secure way.

## How to use with Weave Flux Pod?

Complete the [Deployment Guide](#deployment), noting the secret name and then follow [Flux Guide](https://github.com/bzon/ecr-k8s-secret-creator/blob/master/FLUX_GUIDE.md).

## How to Deploy

<!-- vim-markdown-toc GFM -->

* [IAM Role Requirement](#iam-role-requirement)
* [Deployment](#deployment)
* [Check the ECR Secret Creator Pod's logs](#check-the-ecr-secret-creator-pods-logs)
* [Check the Created Secret](#check-the-created-secret)

<!-- vim-markdown-toc -->

### IAM Role Requirement

If you are __NOT__ using [kube2iam](https://github.com/jtblin/kube2iam), skip to the [Deployment Step](#deployment) and ensure that your Pod's EC2 instance can authenticate to your AWS Account via AWS API Keys, or AWS EC2 IAM Role.

__Create IAM Policy__

```json
{
    "Version": "2012-10-17",
    "Statement": [
        {
            "Effect": "Allow",
            "Action": "ecr:GetAuthorizationToken",
            "Resource": "*"
        }
    ]
}
```

```bash
aws iam create-policy --policy-name ${GET_ECR_AUTH_IAM_POLICY} --policy-document file://iam-policy.json --description "A policy that can get ECR authorization token"
```

__Create IAM Role__

```json
{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Effect": "Allow",
      "Principal": {
        "Service": "ec2.amazonaws.com"
      },
      "Action": "sts:AssumeRole"
    },
    {
      "Sid": "",
      "Effect": "Allow",
      "Principal": {
        "AWS": "arn:aws:iam::${AWS_PROFILE}:role/${IAM_K8S_NODE_ROLE}"
      },
      "Action": "sts:AssumeRole"
    }
  ]
}
```

```bash
aws iam create-role --role-name ${GET_ECR_AUTH_IAM_ROLE} \
  --assume-role-policy-document file://ec2-iam-trust.json
```

__Attach the IAM Policy__

```bash
aws iam attach-role-policy --role-name ${GET_ECR_AUTH_IAM_ROLE} \
  --policy-arn arn:aws:iam::${AWS_PROFILE}:policy/${GET_ECR_AUTH_IAM_POLICY}
```

### Deployment

Create the following RBAC and deploy resources yaml files and run `kubectl apply -f`.

```yaml
---
kind: ServiceAccount
apiVersion: v1
metadata:
  name: ecr-k8s-secret-creator
  namespace: ${SECRET_NAMESPACE}
---
kind: ClusterRole
apiVersion: rbac.authorization.k8s.io/v1
metadata:
  name: ecr-k8s-secret-creator
rules:
- apiGroups: [""]
  resources: ["secrets"]
  verbs: ["get", "list", "watch", "create", "update", "patch", "delete"]
---
kind: ClusterRoleBinding
apiVersion: rbac.authorization.k8s.io/v1
metadata:
  name: ecr-k8s-secret-creator
roleRef:
  kind: ClusterRole
  name: ecr-k8s-secret-creator
  apiGroup: rbac.authorization.k8s.io
subjects:
- kind: ServiceAccount
  name: ecr-k8s-secret-creator
  namespace: ${SECRET_NAMESPACE}
```

```yaml
apiVersion: extensions/v1beta1
kind: Deployment
metadata:
  name: ecr-k8s-secret-creator
  namespace: ${SECRET_NAMESPACE}
spec:
  replicas: 1
  selector:
    matchLabels:
      app: ecr-k8s-secret-creator
  template:
    metadata:
      labels:
        app: ecr-k8s-secret-creator
        # if using kube2iam
        # annotations:
        # iam.amazonaws.com/role: arn:aws:iam::${AWS_PROFILE}:role/${GET_ECR_AUTH_IAM_ROLE}
    spec:
      serviceAccount: ecr-k8s-secret-creator
      containers:
        - image: bzon/ecr-k8s-secret-creator:latest
          name: ecr-k8s-secret-creator
          args:
            - "-secretName=ecr-docker-secret"
            - "-region=us-east-1"
            # the default profile is the AWS account where the kubernetes cluster is running
            # - "-profile=${AWS_ACCOUNT_ID}"
            # the default interval for fetching new ecr token is 200 seconds
            # - "-interval=300"
          resources:
            requests:
              cpu: 10m
              memory: 32Mi
            limits:
              cpu: 50m
              memory: 64Mi
```

### Check the ECR Secret Creator Pod's logs

```bash
kubectl get secrets -n ${SECRET_NAMESPACE}

Creating secret..
{
 "metadata": {
  "name": "${SECRET_NAME}",
  "creationTimestamp": null
 },
 "data": {
  "config.json": "${BASE64_ENCODED_DOCKER_AUTH_CONFIG"
 }
}
Created a secret "ecr-docker-secret".
```

### Check the Created Secret

```bash
kubectl get secrets ecr-docker-secret -n ${SECRET_NAMESPACE} -o json | jq '.data["config.json"]' | tr -d '"' |  base64 --decode


{
  "auths": {
	 "https://${AWS_PROFILE}.dkr.ecr.us-east-1.amazonaws.com": {
	   "auth": "....."
	 }
  }
}
```
