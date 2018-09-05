[![Go Report Card](https://goreportcard.com/badge/github.com/bzon/ecr-k8s-secret-creator)](https://goreportcard.com/report/github.com/bzon/ecr-k8s-secret-creator)

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
              memory: 64Mi
            limits:
              cpu: 100m
              memory: 128Mi
```

### Check the ECR Secret Creator Pod's logs

```bash
kubectl get secrets -n ${SECRET_NAMESPACE}

Creating secret..
{
 "metadata": {
  "name": "ecr-docker-secret",
  "creationTimestamp": null
 },
 "data": {
  "config.json": "ewogICJhdXRocyI6IHsKCSAiaHR0cHM6Ly85MDM5NjU0ODc1OTQuZGtyLmVjci51cy1lYXN0LTEuYW1hem9uYXdzLmNvbSI6IHsKCSAgICJhdXRoIjogIlFWZFRPbVY1U25kWldHeHpZakpHYTBscWIybGpibHBOWWtjNVMyVklVa1pOVkd4WVpESldWR0pXUW5SVGExWkxXakE1TlZKVk5IcGpNV2hWVTJwb2NVOVlaRWxsVmtrMFkxY3hVMUp1Umt0bFUzUTJWVEJTVUZac1ZqSmxiVkpPVXpKYU5WUkVUalJqZW1ReVpGaG9jMVZGV2tWaVZUVldXV3RzYWxFeWVGZE5WV3d3WlZjMWFGRnVjRU5pUkd4UFRWUnNVVTlVWkVOVFIwcHZVekZvTlZNd1JYcFdhMHBvWWxjMU1tRkZUbEZQU0U1elUxZDNOVXN5TldsV01WWXdWMnhLWVZSVmRFMVdNbEpSVTBSV1VVMHdlRTFNTWpWWFVWWnZjbE5GYjNaWFJtTXhZMnN4ZDFsWVpFaFdWbFp1WVRJeFdsWjZhSFJsVm1oM1RESlZNMkpWY0RaWFZYUXpWbGRvVDFSclRuUlhSMHB4VWxjeFRXUnNVVFZpYW1SaFZtcGtVRmt3U2twVGJVcFFWRVJXVDFSRWJETlphMVkwWkROb2EyRnRXakJQVjNjMVRVVm9lV0ZyVW14amF6VkpUVzFvVjFaRE9IaFBTR2hZVDBoU2RFd3hXazVWZWtWNVlWZEdTVk5EZEVSaFJYUkVZekl4ZWxaWVNucGFhbGw2VTFkd1ZFc3piREZpTUc5NFUwaFNWazVzU1RKbFZuQnVWMGRvVDFJd2JHeFJhMGwzVXpBeGMxSlZlSEpQVlVaNVpVUnNVVk5HUm01VVJGcGFWRVp3Vm1WdVZqSmpiV3hGVVc1a1NGRXhaRXBpYlZZeFZVVkthVmRZV2tWaFJYUTFWa2RvWVZSdVVsTmhNRlY0VGpBeGRGWkZjSGRSVms1NFlVVktTRk13V2tWaFYwbzJVMVJHTkdWSVZYcGxSVEY0WkcwMU1WUklSVEJhVjFvelpFVjNNbFZFU2taYU1EbFdaRWRvU0dWWFVUVk9NVTE2VFZoV2JVMVZhelJoUm1jeldtdGFRMDVJYkhObFJYUnNUVEE1VTJGNWREWmpWekZFVWtkMGNWTkZjM2xqZW1oM1RESlZlV05XWTNwVmVrbHlWbXRrYjA5SWIzWlJNR2d3WkcwNU1sSjVPSGhPVmxwRVV6RldibHBzVG14TlIyeEVZVEJuTTFaNlRYZFZSM0JWV1ZkR1RsTkljRU5WTTFJeVdsaHJlbGx1VWxaYVZGSlBUVzFvVjAxc1RsVk9Semd5V2tSak0ySlhkRmRUUjJkM1REQmtWMU5IYUVkbFdGVXdaVlZTUkU1Rk1UUlpXRkpZVTBkR1NtSkVWa05PYVhSSFYydEtiRkV4VGtsTk1WcHVWVWRrTkdWV1VtMVBSVVpTWlVaS1JsSkVXbmxaVjBaSlVUTk9SMWxxYUVKT1JWcFpWMjFPTWxkc1dURmphbXhMVjIwMVRWcFhhekJYUlZJeFVWWm9WMVJHY0ZKYU0yeFVWa1JPU21GWE1XRlNNVW93VERKT01GUlZVbEJWVlZwRldrYzFSR1ZIUm5wYU1teElUMGRTV0VzeVZtNVJWVkY1WW5rNGQwOVhOSFphVlhRelV6TnNNRTlWT1VWa2JYUllaVVpzTTJWSWJIcGFNSE14WTBkV1NWSlhNV0ZrZWtFeVdXdEpORlZYY3pKTlZYUkVaVzFhTlZkWVRuaFhiRnBQVFZaV1VsbFhiM1pTTWxadFlYcG5kMk5GZUROVFYyeDFWR3MxUTFwVk5VSldiVFZHVjBkNFJWcEZSbWhTVlZwS1ZESTFjRlF5Vlhaa01uQnNUakZXUTFwcVduaGpiVXBZVTFoc1FrNVlSWFpsUmxaVFlVWmFSMlZ1UWxKamJsSjRXVzFzU1ZwclZYSmtNbkIzWWxSVmRsUldSakJrUkd4R1ZtdDRjV1ZyUm5aWGJsWjVZbXRTZWxwVk1YSk1NMjl5WVVWNFJHVkZPWEpWTVdzeFdrUmtlbUV6UWtaVGVrWlVWVEZhVTAxdVNUQlZibkI2VkRBNWFFMHdTalZTTWpWSldtNUthMDV0V25WaFIxa3pZMFZhVjFSV2FITmxWVFV6VG0xd1ZXUnNRVEpVTUZaNFZqRkdNMVJGU25CTE1rcFVaRlpqTWxSWVduQlRWemxyWkhwR2NVNHlWa1ZTYmtKdlZXMVdXbHA1ZERSTk1WSnVXbTFhVWxKVWFIaGxiRlowVWtoS1ExcFhjRTFqYlRFeFlXeHNWVlZ0V1RWV1dFWkNUa1paZVdJeU1USmhiWE40VkZkT1ExcFhiRkpWUjBZd1RucHNlbFZyVm1sVGJrMTVUbGRhYzFsc1FqUlRNMEpLWkdzMGRsZFhiR3hhUjNCQ1ZsUnNibVJXVW5oVVNFcERZMVZ3ZUZKSVZqWlBWVEZxVTFWYU1GTlhNVFJMTUZKdFdtNUdRMDFXYkc1U2JrWjBZVEJPYkZWWVNqSmliVEF4VVZkU1IxcElTakZTYTJ0NlZVWnJOVkpIT1hCYWJUVkNUakp6ZGxaWVFsUmtSV3R5VmxVNVVsVkhhR3RPYldSWVdXdG9WRTlHWkhOaVZFWlVaVlJXY1ZkdVVtcGtNRlphWkRGYWExSnJOVFpUUmxrd1pHeENRbEZWYXpOVmJHUnBVV3BzTW1GcVZtcE9SbkJEVTFVME0xbHFaSFpYYkhCS1dWUk9jMDF1Um01TlIyTjZUbTV3VDFrd2VERlBTRkp4V210c2IxZEhkRWxaV0VZeFpWaFJOVlY2UWtsT2JIQnpUa1JzUTJRd05WZGtRM1JSVmxST2QyVllXbWxaVmxveVl6SldVVlJxWkRWWFJrb3lVbGhuZWsweVpETmpTR2hHV210T1RGcFdUbTFOUkdSNllrUlJlbGxXVVROV2VtTjVUbTFrTVZWdVFsWk9hMmg1VGpKb2VFMXFWakJWYm1SVll6QkdUMVo2VmtsbFZtdDJUMGhGTUZWWGVHeFRXRVkyVERJMVYxbHRPVzlOYlU1MlpGWkdUMk5ZY0hOU2JrNWFXVE53Y0ZGclpGbFBTRVpXWkRGT1dFMHhUalpVUkVwUVpVTnpjbGt3TVhOWFJHeGhUa1ZXY1dKRk5ETmtlbWR5WkZkU1JWcHJSVEpqVnpseVdrZHNTMVJZUm5kU1ZXaGFWVEZKTTA1dFRrSlZNMG94WkZVMWJHTlVTblJrVkdSMFRqQTFOazlXV2tka2VqQTVTV2wzYVZwSFJqQlpWM1JzWlZOSk5rbHJSbEpTVlVwQ1UwZG9NMkpVUWxwWlZXeFVVMjFXVTJSRmNIUk9WelI0VW5wYU1XTlhWbXhoTVdneFlqRm9XVlZIVlRGV1ZWcHFXbFJzVTJOVVozWk5WRkl6VVZWR1FsTkVVak5hYTBaYVUydDBkbGRyYkc5a2JVNVBVVlpHYWxJeU9VaFBTR1JwVlZWc1ExRlZVa05pTUVwdVlUTkdiMkV5YkVoUFdHTjNVV3RLTTFKWVpFbGFNV3hMVjFWc1lWTlZSbGhXVlZKRFVWVldNVlJWU2taU1ZWSk9UMFphZFZNeVozWmxXR1JLVFVkU1NHUklUbGhWVld4RFVsVnNRazR4V2xSalNFcFlZMnMxVG1Jd1drWmhiRVozVVZkNE5tUllRa2xXYkZwMlpVZHplRkl5UlRWalNGSTJWbXBrV2xveU9VVlNNV3hyV2tSV1IwMHlNSGxQUnpGT1dWY3hVVlJyZUhSak0wVjRZV3Q0YjJWcVFYaFdiRVl6VjFVd01tSnVaRXBUTVZwVlUwaE5PVWxwZDJsa2JWWjVZekpzZG1KcFNUWkpha2xwVEVOS01HVllRbXhKYW05cFVrVkdWVkZXT1V4U1ZtdHBURU5LYkdWSVFuQmpiVVl3WVZjNWRVbHFiM2hPVkUweFRtcG5NRTU2UVhobVVUMDkiCgkgfQogIH0KfQ=="
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
