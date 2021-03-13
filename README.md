
```shell
kubectl create clusterrolebinding default-view --clusterrole=view --serviceaccount=default:default
kubectl run --rm -i demo --image=test
```
