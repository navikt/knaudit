package main

import (
	"bufio"
	"context"
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"regexp"
	"strings"
	"time"

	goora "github.com/sijms/go-ora/v2"

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

	auditData, err := getAuditData()
	if err != nil {
		log.Error(err)
		panic(err)
	}

	marshalAuditData, err := json.Marshal(auditData)
	if err != nil {
		log.Error(err)
		panic(err)
	}

	log.Info(string(marshalAuditData))

	err = sendAuditDataToDVH(string(marshalAuditData))
	if err != nil {
		log.Error(err)
		panic(err)
	}
}

func sendAuditDataToDVH(blob string) error {
	connection, err := goora.NewConnection(os.Getenv("ORACLE_URL"))
	if err != nil {
		return fmt.Errorf("failed creating new connection to Oracle: %v", err)
	}

	err = connection.Open()
	if err != nil {
		return fmt.Errorf("failed opening connection to Oracle: %v", err)
	}

	defer connection.Close()

	stmt := goora.NewStmt("begin dvh_vpd_adm.als_api.log(p_event_document => :1); end;", connection)
	stmt.AddParam("1", &blob, 0, goora.Input)
	result, err := stmt.Exec([]driver.Value{})
	if err != nil {
		return fmt.Errorf("failed executing statement: %v", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return err
	}

	if rowsAffected != 1 {
		return fmt.Errorf("there where %v rows affected, should only be 1", rowsAffected)
	}

	return nil
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
	if err != nil {
		return nil, err
	}
	auditData["triggered_by"] = triggeredBy

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
	err = db.QueryRow(context.Background(), `SELECT owner FROM public.log WHERE dag_id = $1 
                               AND event = 'trigger' ORDER BY dttm DESC LIMIT 1;`, dagID).Scan(&owner)
	if err != nil {
		if err == pgx.ErrNoRows {
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
