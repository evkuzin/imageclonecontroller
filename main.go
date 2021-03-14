package main

import (
    "context"
    "flag"
    "fmt"
    "github.com/docker/distribution/uuid"
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

type storage interface {
    CheckImage(image string) (string, bool)
    PutImage(old, new string)
}

type event struct {
    eventType        string // Its easier to have one more field than doing reflection each time
    eventObj         watch.Event
    ContainersOrigin []v12.Container
    ContainersTODO   map[*name.Reference]*string
    Storage          storage
    EventID          uuid.UUID
}

const (
    deployment = "deployment"
    daemonset  = "daemonset"
    kubesystem = "kube-system"
)

var (
    defaultRepo     *string
    defaultRegistry *string
    logLevel        *string
)

func main() {
	log.SetFormatter(&log.TextFormatter{
		ForceColors:      true,
		DisableTimestamp: false,
		FullTimestamp:    true,
		PadLevelText:     true,
		QuoteEmptyFields: true,
	})
	log.SetReportCaller(true)
	logLevel = flag.String("log", "info", "logging level")
	defaultRegistry = flag.String("reg", "index.docker.io", "registry location")
	defaultRepo = flag.String("repo", "evkuzin", "repository location")
	switch *logLevel {
	case "info":
		log.SetLevel(log.InfoLevel)
		log.SetReportCaller(false)
	case "debug":
		log.SetLevel(log.DebugLevel)
	default:
		log.Infof("wrong level setting '%s', set level to info", *logLevel)
	}
    flag.Parse()

    log.Info("Starting...")
    newController()
}

func newController() {
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

    for {
        event := &event{
            eventType:        "",
            eventObj:         watch.Event{},
            ContainersOrigin: nil,
            ContainersTODO:   nil,
            Storage:          newInMemoryStorage(),
            EventID:          uuid.Generate(),
        }

        select {
        case event.eventObj = <-ds.ResultChan():
            event.eventType = daemonset
        case event.eventObj = <-deploys.ResultChan():
            event.eventType = deployment
        }

        go event.CheckImage().ParseImages().PushImage().RefactorManifest(clientset)
    }
}

func (e *event) CheckImage() *event {
    if e.eventObj.Type == watch.Modified || e.eventObj.Type == watch.Added {
        switch e.eventType {
        case daemonset:
            temp := e.eventObj.Object.(*v1.DaemonSet)
            if temp.Namespace == kubesystem {
                log.WithFields(log.Fields{
                    "EventID":    e.EventID.String(),
                    "deployment": temp.Name,
                    "namespace":  temp.Namespace,
                    "msg":        "Skipping...",
                }).Debug()
                return e
            }
            e.ContainersOrigin = temp.Spec.Template.Spec.Containers
        case deployment:
            temp := e.eventObj.Object.(*v1.Deployment)
            if temp.Namespace == kubesystem {
                log.WithFields(log.Fields{
                    "EventID":    e.EventID.String(),
                    "deployment": temp.Name,
                    "namespace":  temp.Namespace,
                    "msg":        "Skipping...",
                }).Debug()
                return e
            }
            e.ContainersOrigin = temp.Spec.Template.Spec.Containers
        }
    }
    return e
}

func (e *event) ParseImages() *event {
    if e.ContainersOrigin != nil {
        e.ContainersTODO = make(map[*name.Reference]*string)
        for idx, currentImage := range e.ContainersOrigin {
            ref, err := name.ParseReference(currentImage.Image)
            if err != nil {
                panic(err)
            }
            repo := ref.Context().RepositoryStr()
            registry := ref.Context().RegistryStr()
            if strings.Split(repo, "/")[0] != *defaultRepo || registry != *defaultRegistry {
                log.WithFields(log.Fields{
                    "EventID":  e.EventID.String(),
                    "msg":      "New container TODO",
                    "registry": registry,
                    "image":    repo,
                }).Info()
                e.ContainersTODO[&ref] = &e.ContainersOrigin[idx].Image
            } else {
                log.WithFields(log.Fields{
                    "EventID":  e.EventID.String(),
                    "msg":      "Skipping container",
                    "registry": registry,
                    "image":    repo,
                }).Debug()
            }
        }
    }
    return e
}

func (e *event) PushImage() *event {
    if e.ContainersTODO != nil {
        for ref := range e.ContainersTODO {
            if val, ok := e.Storage.CheckImage(*e.ContainersTODO[ref]); !ok {
                img, err := remote.Image(*ref, remote.WithAuthFromKeychain(authn.DefaultKeychain))
                if err != nil {
                    log.WithFields(log.Fields{
                        "EventID": e.EventID.String(),
                        "msg":     "Failed to get remote image reference",
                        "err":     err,
                    }).Error()
                    continue
                }
                imgTag, err := img.Digest()
                if err != nil {
                    log.WithFields(log.Fields{
                        "EventID": e.EventID.String(),
                        "msg":     "Failed to get image digest",
                        "err":     err,
                    }).Error()
                    continue
                }
                imgName := strings.Split((*ref).Context().RepositoryStr(), "/")[1]
                tag, err := name.NewTag(fmt.Sprintf("%s/%s/%s:%s", *defaultRegistry, *defaultRepo, imgName, imgTag.Hex))
                if err != nil {
                    log.WithFields(log.Fields{
                        "EventID": e.EventID.String(),
                        "msg":     "Failed to create a new tag",
                        "err":     err,
                    }).Error()
                    continue
                }
                err = remote.Write(tag, img, remote.WithAuthFromKeychain(authn.DefaultKeychain))
                if err != nil {
                    log.WithFields(log.Fields{
                        "EventID": e.EventID.String(),
                        "msg":     "Failed to push image",
                        "err":     err,
                    }).Error()
                    continue
                }
                log.WithFields(log.Fields{
                    "EventID": e.EventID.String(),
                    "msg":     "Image uploaded successfully",
                    "Image":   tag.Name(),
                }).Info()
                e.Storage.PutImage(*(e.ContainersTODO[ref]), tag.Name())
                *e.ContainersTODO[ref] = tag.Name()
            } else {
                *e.ContainersTODO[ref] = val
                log.WithFields(log.Fields{
                    "EventID":  e.EventID.String(),
                    "msg":      "Image was upload before. Use uploaded image",
                    "OldImage": (*ref).Name(),
                    "NewImage": val,
                }).Info()
            }
        }
    } else {
        log.WithFields(log.Fields{
            "EventID": e.EventID.String(),
            "msg":     "No containers to proceed",
        }).Debug()
    }
    return e
}

func (e *event) RefactorManifest(clientset *kubernetes.Clientset) {
    if e.ContainersTODO == nil || len(e.ContainersTODO) == 0 {
        log.WithFields(log.Fields{
            "EventID": e.EventID.String(),
            "msg":     "No containers to proceed",
        }).Debug()
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
                log.WithFields(log.Fields{
                    "EventID": e.EventID.String(),
                    "msg":     "Failed to get latest version of DaemonSet",
                    "err":     getErr,
                }).Error()
                return getErr
            }
            result.Spec.Template.Spec.Containers = e.ContainersOrigin
            _, updateErr := DaemonSetsClient.Update(context.TODO(), result, metav1.UpdateOptions{})
            return updateErr
        })
        if retryErr != nil {
            log.WithFields(log.Fields{
                "EventID": e.EventID.String(),
                "msg":     "Update failed",
                "err":     retryErr,
            }).Error()
        }
        log.WithFields(log.Fields{
            "EventID":   e.EventID.String(),
            "msg":       "DaemonSet was updated",
            "DaemonSet": dname,
            "namespace": namespace,
        }).Info()
    case deployment:
        namespace := e.eventObj.Object.(*v1.Deployment).Namespace
        dname := e.eventObj.Object.(*v1.Deployment).Name
        deploymentsClient := clientset.AppsV1().Deployments(namespace)
        retryErr := retry.RetryOnConflict(retry.DefaultRetry, func() error {
            result, getErr := deploymentsClient.Get(context.TODO(), dname, metav1.GetOptions{})
            if getErr != nil {
                log.WithFields(log.Fields{
                    "EventID": e.EventID.String(),
                    "msg":     "Failed to get latest version of Deployment",
                    "err":     getErr,
                }).Error()
                return getErr
            }

            result.Spec.Template.Spec.Containers = e.ContainersOrigin
            log.WithFields(log.Fields{
                "EventID":            e.EventID,
                "ContainersInit":     result.Spec.Template.Spec.Containers,
                "ContainersModified": e.ContainersOrigin,
            }).Debug()
            _, updateErr := deploymentsClient.Update(context.TODO(), result, metav1.UpdateOptions{})
            return updateErr
        })
        if retryErr != nil {
            log.WithFields(log.Fields{
                "EventID": e.EventID.String(),
                "msg":     "Update failed",
                "err":     retryErr,
            }).Error()
        }
        log.WithFields(log.Fields{
            "EventID":    e.EventID.String(),
            "msg":        "Deployment was updated",
            "Deployment": dname,
            "namespace":  namespace,
        }).Info()
    }
}
