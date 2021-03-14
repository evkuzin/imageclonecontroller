The idea is to subscribe to the k8s events,
scan containers in daemonsets and deployments, upload public images to default registry 
and replace images with the new one from our location.


To compile and run:
- check manifests (I did tests with docker registry and it require auth, 
so I added docker conf with auth, but you probbaly need different settings here)
- by default, it will be created in default namespace
- run

```shell
export IMAGELOADER=k8s.gcr.io/imageloader:latest
export CONTEXT=YOUR_K8S_CONTEXT 
make docker
```


TODO:
- create tests
- add retries in case of image error