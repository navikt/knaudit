package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/elastic/go-elasticsearch/esapi"
	"github.com/elastic/go-elasticsearch/v8"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/joho/godotenv"
	log "github.com/sirupsen/logrus"
)

func main() {
	log.SetFormatter(&log.JSONFormatter{})

	localEnv := godotenv.Load(".env") == nil
	if localEnv {
		log.Info(".env file found, application is configured to run locally.")
	}

	elasticSearchClient, err := configureElasticSearch(localEnv)
	if err != nil {
		panic(err)
	}

	auditData, err := getAuditData()
	if err != nil {
		panic(err)
	}

	index := os.Getenv("ELASTICSEARCH_INDEX")
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
		cafilePath := os.Getenv("CA_CERT_PATH")
		cacertBytes, err := os.ReadFile(cafilePath)
		if err != nil {
			return nil, fmt.Errorf("failed to open ca certificate file %v: %v", cafilePath, err)
		}
		cfg.CACert = cacertBytes
	}

	client, err := elasticsearch.NewClient(cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to create elastic search client %v", err)
	}

	return client, nil
}

func getAuditData() (map[string]string, error) {
	var err error
	auditData := make(map[string]string)

	auditData["hostname"], err = os.Hostname()
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

	triggeredBy, err := getTriggeredBy(auditData["dag_id"], auditData["run_id"])
	auditData["triggered_by"] = triggeredBy

	repoPath := os.Getenv("GIT_REPO_PATH")
	auditData["commit_sha1"], err = getGitCommitSHA1(repoPath)
	if err != nil {
		return nil, err
	}

	auditData["git_repo"], err = getGitRepo(repoPath + "/" + ".git/config")
	if err != nil {
		return nil, err
	}

	auditData["@timestamp"], err = extractDate(auditData["run_id"])
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
	return "", fmt.Errorf("no ip address found")
}

func getTriggeredBy(dagID, runID string) (string, error) {
	if strings.HasPrefix(runID, "scheduled") {
		return "airflow", nil
	}

	ctx := context.Background()
	dbURL := os.Getenv("AIRFLOW_DB_URL")
	db, err := pgx.Connect(ctx, dbURL)
	if err != nil {
		panic(err)
	}
	defer db.Close(ctx)

	var owner string
	err = db.QueryRow(context.Background(), `SELECT owner FROM public.log WHERE dag_id = $1 
                               AND event = 'trigger' ORDER BY dttm DESC LIMIT 1;`, dagID).Scan(&owner)
	if err != nil {
		panic(err)
	}

	return owner, nil
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

func getGitRepo(gitConfigPath string) (string, error) {
	gitConfigFile, err := os.Open(gitConfigPath)
	if err != nil {
		return "", err
	}

	defer gitConfigFile.Close()
	gitRepoRegexp := regexp.MustCompile(`(?P<name>github\.com\/(navikt|nais)\/.+)`)

	scanner := bufio.NewScanner(gitConfigFile)
	for scanner.Scan() {
		line := scanner.Text()
		if gitRepoRegexp.MatchString(line) {
			repo := gitRepoRegexp.FindStringSubmatch(line)[1]
			return repo, nil
		}
	}

	return "", scanner.Err()
}

func extractDate(runID string) (string, error) {
	regex := regexp.MustCompile("\\d{4}-\\d{2}-\\d{2}T\\d{6}")
	date := regex.FindString(runID)
	if date == "" {
		return "", fmt.Errorf("failed to extract from runID %v", runID)
	}

	parsedTime, err := time.Parse("2006-01-02T150405", date)
	if err != nil {
		return "", err
	}

	return parsedTime.Format(time.RFC3339), nil
}

func sendToKibana(es *elasticsearch.Client, index string, auditData map[string]string) error {
	reqBody, err := json.Marshal(auditData)
	if err != nil {
		return fmt.Errorf("error marshaling audit data: %s", err)
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
		return fmt.Errorf("error getting response: %s", err)
	}

	defer res.Body.Close()

	if res.IsError() {
		return fmt.Errorf("[%s] error indexing document ID=%v", res.Status(), documentID)
	} else {
		var bodyMap map[string]interface{}
		if err := json.NewDecoder(res.Body).Decode(&bodyMap); err != nil {
			return fmt.Errorf("error parsing the response body: %s", err)
		} else {
			log.Infof("[%s] %s", res.Status(), bodyMap["result"])
		}
	}

	return nil
}
