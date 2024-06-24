package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/joho/godotenv"
	log "github.com/sirupsen/logrus"
)

var retryDelays = []int{1, 3, 5}

func main() {
	log.SetFormatter(&log.JSONFormatter{})

	httpClient := &http.Client{
		Timeout: time.Second * 10,
	}

	if err := godotenv.Load(".env"); err == nil {
		log.Info(".env file found, application has been configured to run locally.")
	}

	auditData, err := getAuditData()
	if err != nil {
		log.Error(err)
		return
	}

	marshalledAuditData, err := json.Marshal(auditData)
	if err != nil {
		log.Error(err)
		return
	}

	for i := 0; i < len(retryDelays); i++ {
		if err := postAuditData(httpClient, marshalledAuditData); err == nil {
			break
		}
		time.Sleep(time.Second * time.Duration(retryDelays[i]))
		log.Info("retrying audit data post to knaudit-proxy...")
	}
}

func getAuditData() (map[string]string, error) {
	var err error
	auditData := make(map[string]string)

	auditData["ip"], err = getLocalIP()
	if err != nil {
		return nil, err
	}

	auditData["hostname"] = os.Getenv("POD_NAME")
	auditData["namespace"] = os.Getenv("NAMESPACE")
	auditData["dag_id"] = os.Getenv("AIRFLOW_DAG_ID")
	auditData["run_id"] = os.Getenv("AIRFLOW_RUN_ID")
	auditData["task_id"] = os.Getenv("AIRFLOW_TASK_ID")

	auditData["triggered_by"], err = getTriggeredBy(auditData["dag_id"], auditData["run_id"])
	if err != nil {
		return nil, err
	}

	repoPath := os.Getenv("GIT_REPO_PATH")

	auditData["commit_sha1"], err = getGitCommitSHA1(repoPath)
	if err != nil {
		return nil, err
	}

	auditData["git_branch"], err = getGitBranch(repoPath)
	if err != nil {
		return nil, err
	}

	auditData["git_repo"], err = getGitRepo(repoPath + "/" + ".git/config")
	if err != nil {
		return nil, err
	}

	auditData["timestamp"] = time.Now().Format(time.RFC3339)

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
		return "", err
	}
	defer db.Close(ctx)

	var owner string
	sqlQuery := `SELECT owner FROM public.log WHERE dag_id = $1 AND event in ('trigger','cli_task_run') ORDER BY dttm DESC LIMIT 1;`
	if err = db.QueryRow(context.Background(), sqlQuery, dagID).Scan(&owner); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", fmt.Errorf("ingen eier for DAG='%v' funnet", dagID)
		}

		return "", err
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

	names, err := file.Readdirnames(1)
	if err != nil {
		return "", err
	}

	name := names[0]
	data, err := os.ReadFile(gcfilePath + "/" + name)
	if err != nil {
		return "", err
	}

	return strings.ReplaceAll(string(data), "\n", ""), nil
}

func getGitBranch(repoPath string) (string, error) {
	gcfilePath := repoPath + "/" + ".git/refs/heads"
	file, err := os.Open(gcfilePath)
	if err != nil {
		return "", err
	}
	defer file.Close()

	names, err := file.Readdirnames(1)
	if err != nil {
		return "", err
	}

	return names[0], nil
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

func postAuditData(httpClient *http.Client, marshalledAuditData []byte) error {
	res, err := httpClient.Post(fmt.Sprintf("%v/report", os.Getenv("KNAUDIT_PROXY_URL")), "application/json", bytes.NewBuffer(marshalledAuditData))
	if err != nil {
		log.WithError(err).Error("posting knaudit data to proxy")
		return err
	}
	defer res.Body.Close()

	bodyBytes, err := io.ReadAll(res.Body)
	if err != nil {
		log.WithError(err).Error("reading response body")
		return err
	}

	if res.StatusCode != http.StatusOK {
		log.Errorf("posting knaudit data to proxy returned status code %v, response: %v", res.StatusCode, string(bodyBytes))
		return err
	}

	return nil
}
