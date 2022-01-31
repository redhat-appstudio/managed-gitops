package db

import (
	"bytes"
	"fmt"
	"log"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/google/uuid"
	"k8s.io/apimachinery/pkg/util/wait"
)

type EphemeralDB struct {
	db            *PostgreSQLDatabaseQueries
	dbContainerID string
	dockerNetwork string
}

// NewEphemeralCreateTestFramework will create a docker container,
// network, connect to database and populate the schema. This is
// merely defined for testing purpose
func NewEphemeralCreateTestFramework() (EphemeralDB, error) {
	dockerName := "managed-gitops-postgres-test"
	dockerNetworkcmd := "docker network create %s"
	uuid := uuid.New().String()
	tempDBName := "db-" + uuid
	tempNetworkName := "gitops-net-" + uuid
	s := fmt.Sprintf(dockerNetworkcmd, tempNetworkName)
	// To print which command is running
	fmt.Println("\nRunning: ", s)

	// #nosec G204
	dockerNetwork := exec.Command("docker", "network", "create", tempNetworkName)
	dockerNetworkerr := dockerNetwork.Run()
	if dockerNetworkerr != nil {
		log.Fatal(dockerNetworkerr)
	}

	tempDatabaseDircmd := "mktemp -d -t postgres-XXXXXXXXXX"
	s = fmt.Sprintf(tempDatabaseDircmd)
	fmt.Println("\nRunning: ", s)

	// To actually run the command (runs in background)
	tempDatabaseDir, err_run := exec.Command("mktemp", "-d", "-t", "postgres-XXXXXXXXXX").Output()
	if err_run != nil {
		log.Fatal(err_run)
	}

	// running a docker container
	dockerContainerIDcmd := `docker run --name ` + dockerName + ` \
	-v ` + string(tempDatabaseDir) + `:/var/lib/postgresql/data:Z \
	-e POSTGRES_PASSWORD=gitops \
	-e POSTGRES_DB=` + tempDBName + ` \
	-p 6432:5432 \
	--network ` + tempNetworkName + ` \
	-d \
	postgres:13 \
	-c log_statement='all' \
	-c log_min_duration_statement=0`

	fmt.Println("\nRunning:", dockerContainerIDcmd)

	var dockerContainerID []byte
	var errDockerRun error
	errWait := wait.Poll(5*time.Second, 2*time.Minute, func() (bool, error) {
		// #nosec G204
		dockerContainerID, errDockerRun = exec.Command("docker", "run", "--name", dockerName,
			"-v", string(tempDatabaseDir)+":/var/lib/postgresql/data:Z",
			"-e", "POSTGRES_PASSWORD=gitops",
			"-e", "POSTGRES_DB="+tempDBName,
			"-p", "6432:5432",
			"--network", "gitops-net-"+uuid,
			"-d",
			"postgres:13",
			"-c", "log_statement=all",
			"-c", "log_min_duration_statement=0").Output()

		if errDockerRun != nil {
			log.Fatal(errDockerRun)
		}
		if dockerContainerID == nil {
			return false, errDockerRun
		}
		// check for container status
		// #nosec G204
		status, _ := exec.Command("docker", "container", "inspect", "-f", "{{.State.Status}}", string(dockerContainerID)).Output()
		if string(status) == "running" {
			return true, nil
		}
		// Todo: Once verified removed these print statements in order to maintain code coverage
		fmt.Println("Docker Container ID: " + string(dockerContainerID))
		return true, nil
	})
	if errWait != nil {
		log.Fatal("error in executing docker run command: ", errWait)
	}

	dbcmd := "PGPASSWORD=gitops psql -h localhost -d %s -U postgres -p 6432 -c 'select 1'"
	s = fmt.Sprintf(dbcmd, tempDBName)

	fmt.Println("\nRunning: ", s)
	// To get the output of the command
	errWait = wait.Poll(5*time.Second, 2*time.Minute, func() (bool, error) {
		// #nosec G204
		psqlcmd := exec.Command("psql", "-h", "localhost", "-d", tempDBName, "-U", "postgres", "-p", "6432", "-c", "select 1")
		psqlcmd.Env = os.Environ()
		psqlcmd.Env = append(psqlcmd.Env, "PGPASSWORD=gitops")
		var outb, errb bytes.Buffer
		psqlcmd.Stdout = &outb
		psqlcmd.Stderr = &errb

		psqlErr := psqlcmd.Run()

		if errb.String() != "" {
			// log.Fatal(errb.String())
			return false, fmt.Errorf(errb.String())
		}
		if psqlErr != nil {
			return false, psqlErr
		}
		fmt.Printf("%s database is ready to use\n", tempDBName)
		return true, nil
	})
	if errWait != nil {
		log.Fatal("error in executing docker run command: ", errWait)
	}

	// creating a new database
	newDBName := "postgres"
	dbcmd = "PGPASSWORD=gitops psql -h localhost -d %s -U postgres -p 6432"
	s = fmt.Sprintf(dbcmd, newDBName)
	fmt.Println("\nRunning: ", s)

	psqlcmd := exec.Command("psql", "-h", "localhost", "-d", newDBName, "-U", "postgres", "-p", "6432")
	psqlcmd.Env = os.Environ()
	psqlcmd.Env = append(psqlcmd.Env, "PGPASSWORD=gitops")
	var errConnection bytes.Buffer
	psqlcmd.Stderr = &errConnection
	psqlErr := psqlcmd.Run()
	if errConnection.String() != "" {
		log.Fatal(errConnection.String())
	}
	if psqlErr != nil {
		log.Fatal("error in creation: ", "\nCommand Error: ", psqlErr, "\nDatabase Error: ", errConnection.String())
	}

	fmt.Printf("the %s database is created and ready to use\n", newDBName)

	// Following command is used to populate the database tables from the db-schema.sql (defined in the monorepo)
	dbcmd = "PGPASSWORD=gitops psql -h localhost -d %s -U postgres -p 6432 -q -f ../../../db-schema.sql"
	s = fmt.Sprintf(dbcmd, newDBName)
	fmt.Println("\nRunning: ", s)
	psqlcmd = exec.Command("psql", "-h", "localhost", "-d", newDBName, "-U", "postgres", "-p", "6432", "-q", "-f", "../../../db-schema.sql")
	var errSchema bytes.Buffer
	psqlcmd.Stderr = &errSchema
	psqlcmd.Env = os.Environ()
	psqlcmd.Env = append(psqlcmd.Env, "PGPASSWORD=gitops")
	psqlErr = psqlcmd.Run()

	if errSchema.String() != "" {
		log.Fatal(errSchema.String())
	}
	if psqlErr != nil {
		log.Fatal(psqlErr)
	}
	fmt.Printf("db schema executed in the %s database\n", newDBName)

	// connect the go code with the database
	database, err := ConnectToDatabase(true, newDBName, 6432)
	if err != nil {
		return EphemeralDB{}, err
	}

	dbq := &PostgreSQLDatabaseQueries{
		dbConnection:   database,
		allowTestUuids: false,
		allowUnsafe:    true,
	}

	fmt.Printf("* WARNING: Unsafe PostgreSQLDB object was created. You should never see this outside of test suites, or personal development.\n")
	res := EphemeralDB{db: dbq, dbContainerID: strings.TrimSpace(string(dockerContainerID)), dockerNetwork: tempNetworkName}

	return res, nil
}

// NewEphemeralCleanTestFramework will clean a docker container,
// and network. This is merely defined for testing purpose
func NewEphemeralCleanTestFramework(dockerContainerID string, tempNetworkName string) error {

	dockerCmd := "docker rm -f %s"
	fmt.Println("\nRunning: ", fmt.Sprintf(dockerCmd, dockerContainerID))

	// To get the output of the command
	_, err := exec.Command("docker", "rm", "-f", dockerContainerID).Output()
	if err != nil {
		log.Fatal(err)
	}

	dockerNetworkcmd := "docker network rm %s"
	fmt.Println("\nRunning: ", fmt.Sprintf(dockerNetworkcmd, tempNetworkName))

	// To get the output of the command
	_, err = exec.Command("docker", "network", "rm", tempNetworkName).Output()
	if err != nil {
		log.Fatal(err)
	}
	return nil
}
