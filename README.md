The idea is to subscribe to the k8s events,
scan containers in daemonsets and deployments, upload public images to default registry 
and replace images with the new one from our location.


To compile and run:

```shell
CONTEXT=YOUR_K8S_CONTEXT make docker
```


TODO:
- create tests
- add retries in case of image error