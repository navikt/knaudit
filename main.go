package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"regexp"

	"github.com/elastic/go-elasticsearch/esapi"
	"github.com/elastic/go-elasticsearch/v8"
	"github.com/google/uuid"
	"github.com/joho/godotenv"
	"github.com/pkg/errors"
)

func main() {
	localEnv := godotenv.Load(".env") == nil
	if localEnv {
		fmt.Println(".env file found, application is configured to run locally.")
	}

	elasticSearchClient, err := configureElasticSearch(localEnv)
	if err != nil {
		panic(err)
	}

	auditData, err := getAuditData()
	if err != nil {
		panic(err)
	}

	index := os.Getenv("ELASTICSEAERCH_INDEX")
	err = sendToKibana(elasticSearchClient, index, auditData)
	if err != nil {
		panic(err)
	}
}

func configureElasticSearch(localEnv bool) (*elasticsearch.Client, error) {
	cfg := elasticsearch.Config{
		Addresses: []string{os.Getenv("ELASTICSEARCH_URL")},
	}

	if localEnv {
		cfg.APIKey = os.Getenv("ELASTIC_API_KEY")
	} else {
		cacertBytes, err := getCACertBytes()
		if err != nil {
			return nil, err
		}
		cfg.CACert = cacertBytes
	}

	es, err := elasticsearch.NewClient(cfg)
	if err != nil {
		return nil, errors.Errorf("Failed to create elastic search client %v", err)
	}

	return es, nil
}

func getCACertBytes() ([]byte, error) {
	cafilePath := os.Getenv("CA_CERT_PATH")
	data, err := os.ReadFile(cafilePath)
	if err != nil {
		return nil, errors.Errorf("Failed to open ca certificate file %v: %v", cafilePath, err)
	}
	return data, nil
}

func getAuditData() (map[string]string, error) {
	var err error
	auditData := make(map[string]string)

	auditData["host_name"], err = os.Hostname()
	if err != nil {
		return nil, err
	}

	auditData["ip"], err = getLocalIP()
	if err != nil {
		return nil, err
	}

	auditData["namespace"] = os.Getenv("NAMESPACE")
	auditData["dag_id"] = os.Getenv("AIRFLOW_DAG_ID")
	auditData["run_id"] = os.Getenv("AIRFLOW_RUN_ID")
	auditData["task_id"] = os.Getenv("AIRFLOW_TASK_ID")

	repoPath := os.Getenv("GIT_REPO_PATH")
	auditData["commit_sha1"], err = getGitCommitSHA1(repoPath)
	if err != nil {
		return nil, err
	}

	auditData["triggered_at"], err = extractDate(auditData["run_id"])
	if err != nil {
		return nil, err
	}

	return auditData, nil
}

// getLocalIP returns the non loopback local IP of the host
func getLocalIP() (string, error) {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return "", err
	}
	for _, address := range addrs {
		// check the address type and if it is not a loopback then return it
		if ipnet, ok := address.(*net.IPNet); ok && !ipnet.IP.IsLoopback() {
			if ipnet.IP.To4() != nil {
				return ipnet.IP.String(), nil
			}
		}
	}
	return "", fmt.Errorf("No ip address found")
}

func getGitCommitSHA1(repoPath string) (string, error) {
	gcfilePath := repoPath + "/" + ".git/refs/heads"
	file, err := os.Open(gcfilePath)
	if err != nil {
		return "", err
	}
	defer file.Close()

	list, _ := file.Readdirnames(1)
	name := list[0]
	data, err := os.ReadFile(gcfilePath + "/" + name)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func extractDate(runID string) (string, error) {
	regex := regexp.MustCompile("\\d{4,}-\\d{2,}-\\d{2,}T\\d{10,}")
	date := regex.FindAllString(runID, 1)
	if len(date) < 1 {
		return "", fmt.Errorf("Failed to extract from runID %v", runID)
	}
	return date[0], nil
}

func sendToKibana(es *elasticsearch.Client, index string, auditData map[string]string) error {
	reqBody, err := json.Marshal(auditData)
	if err != nil {
		return errors.Errorf("Error marshaling audit data: %s", err)
	}

	documentID := uuid.New().String()
	req := esapi.IndexRequest{
		Index:      index,
		DocumentID: documentID,
		Body:       bytes.NewReader(reqBody),
		Refresh:    "true",
	}

	res, err := req.Do(context.Background(), es)
	if err != nil {
		return errors.Errorf("Error getting response: %s", err)
	}
	defer res.Body.Close()

	if res.IsError() {
		return errors.Errorf("[%s] Error indexing document ID=%d", res.Status(), documentID)
	} else {
		var bodyMap map[string]interface{}
		if err := json.NewDecoder(res.Body).Decode(&bodyMap); err != nil {
			return errors.Errorf("Error parsing the response body: %s", err)
		} else {
			fmt.Printf("[%s] %s; version=%d", res.Status(), bodyMap["result"], int(bodyMap["_version"].(float64)))
		}
	}
	return nil
}
