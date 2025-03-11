package collector

import (
	"context"
	"fmt"
	"os"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/clientcmd"
)

type KubernetesCollector struct {
	clientset    *kubernetes.Clientset
	logBasePath  string
	LogCh        chan LogMetaData
	namespace    string
	fileNameList []string
}
type LogMetaData struct {
	ContainerName string
	PodName       string
	PodUID        string
	Namespace     string
	LogPath       string
}

func NewKubernetesCollector(kubeconfig string, logBasePath string, namespace string) (*KubernetesCollector, error) {
	var config *rest.Config
	var err error

	if kubeconfig == "" {
		config, err = rest.InClusterConfig()
	} else {
		config, err = clientcmd.BuildConfigFromFlags("", kubeconfig)
	}
	if err != nil {
		return nil, err
	}

	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, err
	}
	LogData := make(chan LogMetaData, 10)
	// 日志文件目录

	fileNameList := make([]string, 0)

	return &KubernetesCollector{
		clientset:    clientset,
		logBasePath:  logBasePath,
		namespace:    namespace,
		fileNameList: fileNameList,
		LogCh:        LogData,
	}, nil
}

func (k *KubernetesCollector) Start(ctx context.Context) error {
	factory := informers.NewSharedInformerFactory(k.clientset, 30*time.Minute)
	podInformer := factory.Core().V1().Pods().Informer()

	podInformer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			pod := obj.(*corev1.Pod)
			k.handlePodLogPaths(pod)
		},
		UpdateFunc: func(oldObj, newObj interface{}) {
			newPod := newObj.(*corev1.Pod)
			k.handlePodLogPaths(newPod)
		},
	})

	go podInformer.Run(ctx.Done())
	return nil
}

func (k *KubernetesCollector) handlePodLogPaths(pod *corev1.Pod) {
	if pod.Namespace != k.namespace {
		return
	}

	for _, container := range pod.Spec.Containers {
		logPath := fmt.Sprintf("%s/%s_%s_%s%s%s", k.logBasePath, pod.Namespace, pod.Name, string(pod.UID), container.Name, "/", "0.log")
		// 检查日志文件是否存在,不存在跳过

		_, err := os.Stat(logPath)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			fmt.Println("Error checking log file:", err)
			continue
		}
		meta := LogMetaData{
			ContainerName: container.Name,
			PodName:       pod.Name,
			PodUID:        string(pod.UID),
			Namespace:     pod.Namespace,
			LogPath:       logPath,
		}

		k.LogCh <- meta
	}
}
