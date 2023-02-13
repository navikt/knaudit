package main

import (
	"fmt"
	"net"
	"os"
)

func main() {
	hostName, err := os.Hostname()
	if err != nil {
		panic(err)
	}

	ip, err := GetLocalIP()
	if err != nil {
		panic(err)
	}

	namespace := os.Getenv("NAMESPACE")

	dagId := os.Getenv("AIRFLOW_DAG_ID")
	runId := os.Getenv("AIRFLOW_RUN_ID")
	taskId := os.Getenv("AIRFLOW_TASK_ID")

	gcfilePath := os.Getenv("GIT_COMMIT_SHA1_PATH")
	commitSha1, err := getGitCommitSHA1(gcfilePath)

	if err != nil {
		panic(err)
	}

	fmt.Println(hostName)
	fmt.Println(ip)
	fmt.Println(namespace)
	fmt.Println(dagId, runId, taskId)
	fmt.Println(commitSha1)
}

// GetLocalIP returns the non loopback local IP of the host
func GetLocalIP() (string, error) {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return "", err
	}
	for _, address := range addrs {
		// check the address type and if it is not a loopback the display it
		if ipnet, ok := address.(*net.IPNet); ok && !ipnet.IP.IsLoopback() {
			if ipnet.IP.To4() != nil {
				return ipnet.IP.String(), nil
			}
		}
	}
	return "", fmt.Errorf("No ip address found")
}

func getGitCommitSHA1(gcfilePath string) (string, error) {
	file, err := os.Open(gcfilePath)
	if err != nil {
		return "", err
	}
	defer file.Close()

	list, _ := file.Readdirnames(1)
	name := list[0]
	data, err := os.ReadFile(name)
	if err != nil {
		return "", err
	}
	return string(data), nil
}
