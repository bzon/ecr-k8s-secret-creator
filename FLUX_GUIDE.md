
* Create the docker config.json from ECR token. __ecr-k8s-secret-creator already do this for you__

```json
{
  "auths": {
	 "https://${AWS_PROFILE}.dkr.ecr.us-east-1.amazonaws.com": {
	   "auth": "....."
	 }
  }
}
```

* Create the Kubernetes secret from the config.json file. __ecr-k8s-secret-creator already do this for you__

```yaml
apiVersion: v1
type: Secret
metadata:
  name: ecr-docker-secret
  namespace: flux
data:
  config.json: xxxxx # base64 encoded config.json
```

* Use the Kubernetes secret as a docker volume in the Flux pod. Use `kubectl edit deploy flux -n flux` after deploying flux using Helm.

```yaml
apiVersion: extensions/v1beta1
kind: Deployment
.....
spec:
  progressDeadlineSeconds: 600
  replicas: 1
  revisionHistoryLimit: 10
  selector:
    matchLabels:
      app: flux
      release: flux
  strategy:
    rollingUpdate:
      maxSurge: 25%
      maxUnavailable: 25%
    type: RollingUpdate
  template:
    metadata:
      creationTimestamp: null
      labels:
        app: flux
        release: flux
    spec:
      containers:
      - args:
        .....
        # Use the docker-config mount
        - --docker-config=/etc/fluxd/docker/config.json
        image: quay.io/weaveworks/flux:1.5.0
        .....
        # Add a volume mount for docker secret
        volumeMounts:
        .....
        - mountPath: /etc/fluxd/docker
          name: docker-config
      .....
      # Mount the docker secret as a volume
      volumes:
      - name: docker-config
        secret:
          defaultMode: 420
          secretName: ecr-docker-secret
      .....
 ```
