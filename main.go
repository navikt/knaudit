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

	err = sendToKibana(elasticSearchClient, auditData)
	if err != nil {
		panic(err)
	}
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

func sendToKibana(es *elasticsearch.Client, auditData map[string]string) error {
	// Build the request body.
	data, err := json.Marshal(auditData)
	if err != nil {
		return errors.Errorf("Error marshaling document: %s", err)
	}

	d := uuid.New().String()
	// Set up the request object.
	req := esapi.IndexRequest{
		Index:      "tjenestekall-knada-airflow-run-audit",
		DocumentID: d,
		Body:       bytes.NewReader(data),
		Refresh:    "true",
	}

	// Perform the request with the client.
	res, err := req.Do(context.Background(), es)
	if err != nil {
		return errors.Errorf("Error getting response: %s", err)
	}
	defer res.Body.Close()

	if res.IsError() {
		return errors.Errorf("[%s] Error indexing document ID=%d", res.Status(), d)
	} else {
		// Deserialize the response into a map.
		var r map[string]interface{}
		if err := json.NewDecoder(res.Body).Decode(&r); err != nil {
			return errors.Errorf("Error parsing the response body: %s", err)
		} else {
			// Print the response status and indexed document version.
			fmt.Printf("[%s] %s; version=%d", res.Status(), r["result"], int(r["_version"].(float64)))
		}
	}
	return nil
}

func getCACertBytes() ([]byte, error) {
	cafilePath := os.Getenv("CA_CERT_PATH")
	data, err := os.ReadFile(cafilePath)
	if err != nil {
		return nil, errors.Errorf("Failed to open ca certificate file %v: %v", cafilePath, err)
	}
	return data, nil
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

func getAuditData() (map[string]string, error) {
	var err error
	auditData := make(map[string]string)

	auditData["host_name"], err = os.Hostname()
	if err != nil {
		return nil, err
	}

	auditData["ip"], err = GetLocalIP()
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
