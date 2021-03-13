package main

import (
    "context"
    "fmt"
    "github.com/google/go-containerregistry/pkg/authn"
    "github.com/google/go-containerregistry/pkg/name"
    "github.com/google/go-containerregistry/pkg/v1/remote"
    log "github.com/sirupsen/logrus"
    v1 "k8s.io/api/apps/v1"
    v12 "k8s.io/api/core/v1"
    metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
    "k8s.io/apimachinery/pkg/watch"
    "k8s.io/client-go/kubernetes"
    "k8s.io/client-go/rest"
    "k8s.io/client-go/util/retry"
    "strings"
)

func main() {
    log.SetFormatter(&log.TextFormatter{
        DisableColors: true,
        FullTimestamp: true,
    })
    log.Info("Starting...")
    NewController()
}

type Storage interface {
    CheckImage(image string) (string, bool)
    PutImage(old, new string)
}

type event struct {
    eventType        string
    eventObj         watch.Event
    ContainersOrigin []v12.Container
    ContainersTODO   map[name.Reference]*string
    Storage          Storage
}

const (
    defaultRepo     = "evkuzin"
    defaultRegistry = "index.docker.io"
    deployment      = "deployment"
    daemonset       = "daemonset"
    kubesystem     = "kube-system"
)

func NewController() {
    config, err := rest.InClusterConfig()
    if err != nil {
        panic(err.Error())
    }
    clientset, err := kubernetes.NewForConfig(config)
    if err != nil {
        panic(err.Error())
    }
    ds, err := clientset.AppsV1().DaemonSets("").Watch(context.TODO(), metav1.ListOptions{})
    if err != nil {
        panic(err.Error())
    }
    deploys, err := clientset.AppsV1().Deployments("").Watch(context.TODO(), metav1.ListOptions{})
    if err != nil {
        panic(err.Error())
    }
    event := &event{
        eventType:        "",
        eventObj:         watch.Event{},
        ContainersOrigin: nil,
        ContainersTODO:   nil,
        Storage: NewInMemoryStorage(),
    }
    for {
        select {
        case event.eventObj = <-ds.ResultChan():
            event.eventType = daemonset
        case event.eventObj = <-deploys.ResultChan():
            event.eventType = deployment
        }
        go event.CheckImage().ParseImages().PushImage()//.RefactorManifest(clientset)
    }
}

func (e *event) CheckImage() *event {
    if e.eventObj.Type == watch.Modified || e.eventObj.Type == watch.Added {
        switch e.eventType {
        case daemonset:
            temp := e.eventObj.Object.(*v1.DaemonSet)
            if temp.Namespace == kubesystem {
                log.Debugf("DaemonSet %v in %v namespace, skipping...", temp.Name, temp.Namespace)
                return e
            }
            e.ContainersOrigin = temp.Spec.Template.Spec.Containers
            for _, c := range temp.Spec.Template.Spec.Containers {
                log.Infof("DaemonSet %v in %v namespace, container %v with image %v", temp.Name, temp.Namespace, c.Name, c.Image)
            }
        case deployment:
            temp := e.eventObj.Object.(*v1.Deployment)
            if temp.Namespace == kubesystem {
                log.Debugf("Deployment %v in %v namespace, skipping...", temp.Name, temp.Namespace)
                return e
            }
            e.ContainersOrigin = temp.Spec.Template.Spec.Containers
            for _, c := range temp.Spec.Template.Spec.Containers {
                log.Infof("Deployment %v in %v namespace, container %v with image %v", temp.Name, temp.Namespace, c.Name, c.Image)
            }
        }
    }
    return e
}

func (e *event) ParseImages() *event {
    if e.ContainersOrigin != nil {
        e.ContainersTODO = make(map[name.Reference]*string)
        for _, currentImage := range e.ContainersOrigin {
            ref, err := name.ParseReference(currentImage.Image)
            if err != nil {
                panic(err)
            }
            repo := ref.Context().RepositoryStr()
            registry := ref.Context().RegistryStr()
            if strings.Split(repo, "/")[0] != defaultRepo || registry != defaultRegistry {
                log.Infof("New container TODO. registry %v, image %v", registry, repo)
                e.ContainersTODO[ref] = &currentImage.Image
            }
        }
    }
    return e
}

func (e *event) PushImage() *event {
    if e.ContainersTODO != nil {
        for ref := range e.ContainersTODO {
            if val, ok := e.Storage.CheckImage(*e.ContainersTODO[ref]); !ok {
                img, err := remote.Image(ref, remote.WithAuthFromKeychain(authn.DefaultKeychain))
                if err != nil {
                    log.Warnf("Failed to get remote image reference. Err: %s", err)
                }
                imgTag, err := img.Digest()
                if err != nil {
                    log.Warnf("Failed to get image digest. Err: %s", err)
                }
                imgName := strings.Split(ref.Context().RepositoryStr(), "/")[1]
                imgName = "imageloader"
                tag, err := name.NewTag(fmt.Sprintf("%s/%s/%s:%s", defaultRegistry, defaultRepo, imgName, imgTag.Hex))
                if err != nil {
                    log.Warnf("Failed to create a new tag. Err: %s", err)
                }
                err = remote.Write(tag, img, remote.WithAuthFromKeychain(authn.DefaultKeychain))
                if err != nil {
                    log.Warnf("Failed to push image. Err: %s", err)
                }
                log.Infof("Image %s uploaded successfully", tag.Name())
                e.Storage.PutImage(*e.ContainersTODO[ref], tag.Name())
                *e.ContainersTODO[ref] = tag.Name()
                log.Infof("This entity will be updated: %#v", e.eventObj)
            } else {
                log.Infof("Image %v was upload before. Using uploaded image: %v", *e.ContainersTODO[ref], val)
                *e.ContainersTODO[ref] = val
            }
        }
    }
    return e
}

func (e *event) RefactorManifest(clientset *kubernetes.Clientset) {
    if e.ContainersTODO == nil {
        log.Infof("No containers to proceed")
        return
    }
    switch e.eventType {
    case daemonset:
        namespace := e.eventObj.Object.(*v1.DaemonSet).Namespace
        dname := e.eventObj.Object.(*v1.DaemonSet).Name
        DaemonSetsClient := clientset.AppsV1().DaemonSets(namespace)
        retryErr := retry.RetryOnConflict(retry.DefaultRetry, func() error {
            result, getErr := DaemonSetsClient.Get(context.TODO(), dname, metav1.GetOptions{})
            if getErr != nil {
                log.Errorf("Failed to get latest version of DaemonSet: %v", getErr)
            }
            result.Spec.Template.Spec.Containers = e.ContainersOrigin
            _, updateErr := DaemonSetsClient.Update(context.TODO(), result, metav1.UpdateOptions{})
            return updateErr
        })
        if retryErr != nil {
            log.Errorf("Update failed: %v", retryErr)
        }
        log.Infof("DaemonSet %v in %v namespace was updated", dname, namespace)
    case deployment:
        namespace := e.eventObj.Object.(*v1.Deployment).Namespace
        dname := e.eventObj.Object.(*v1.Deployment).Name
        deploymentsClient := clientset.AppsV1().Deployments(namespace)
        retryErr := retry.RetryOnConflict(retry.DefaultRetry, func() error {
            result, getErr := deploymentsClient.Get(context.TODO(), dname, metav1.GetOptions{})
            if getErr != nil {
                log.Errorf("Failed to get latest version of Deployment: %v", getErr)
            }
            result.Spec.Template.Spec.Containers = e.ContainersOrigin
            _, updateErr := deploymentsClient.Update(context.TODO(), result, metav1.UpdateOptions{})
            return updateErr
        })
        if retryErr != nil {
            log.Errorf("Update failed: %v", retryErr)
        }
        log.Infof("Deployment %v in %v namespace was updated", dname, namespace)
    }
}
