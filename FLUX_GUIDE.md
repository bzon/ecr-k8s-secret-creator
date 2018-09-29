## Using with Weave Flux


* Create the docker config.json from ECR token. __ecr-k8s-secret-creator already do this for you__.

```json
{
  "auths": {
	 "https://${AWS_PROFILE}.dkr.ecr.us-east-1.amazonaws.com": {
	   "auth": "....."
	 }
  }
}
```

* Create the Kubernetes secret from the config.json file. __ecr-k8s-secret-creator already do this for you__.

```yaml
apiVersion: v1
type: Secret
metadata:
  name: ecr-docker-secret
  namespace: flux
data:
  config.json: xxxxx # base64 encoded config.json
```

* After deploying Weave Flux with Helm, you must edit the Weave Flux deployment via `kubectl edit deploy flux` and use the created Kubernetes secret as a docker volume in the Flux pod. Please see the [examples](./examples) directory too.

```yaml
apiVersion: extensions/v1beta1
kind: Deployment
.....
spec:
  ...
    spec:
      containers:
      - args:
        .....
        #
        # STEP 3 - Use the config.json inside the volumeMount (/etc/fluxd/docker)
        #
        - --docker-config=/etc/fluxd/docker/config.json
        .....
        volumeMounts:
        #
        # STEP 2 - Create a volumeMount (/etc/fluxd/docker) for the docker-config volume 
        #
        - mountPath: /etc/fluxd/docker
          name: docker-config
          .....
      volumes:
      #
      # STEP 1 - Create a volume named docker-config using your ecr docker secret
      #
      - name: docker-config
        secret:
          defaultMode: 420
          secretName: ecr-docker-secret
        .....
 ```
