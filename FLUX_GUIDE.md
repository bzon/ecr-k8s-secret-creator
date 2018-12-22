## Using with Weave Flux

After deploying Weave Flux with Helm, you must edit the Weave Flux deployment via `kubectl edit deploy flux` and use the created Kubernetes secret by **ecr-k8s-secret-creator** pod as a docker volume in the Flux pod.

In the example below, the secret name is **ecr-docker-secret**.

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
 
 Please see also the [examples](./examples) directory for reference.
