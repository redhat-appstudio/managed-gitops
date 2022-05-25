package db

import (
	"bytes"
	"fmt"
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
	uuid := uuid.New().String()
	tempDBName := "db-" + uuid
	tempNetworkName := "gitops-net-" + uuid

	// #nosec G204
	dockerNetwork := exec.Command("docker", "network", "create", tempNetworkName)
	dockerNetworkerr := dockerNetwork.Run()
	if dockerNetworkerr != nil {
		return EphemeralDB{}, fmt.Errorf("unable to create docker network: %v. Do a 'docker system prune', for now", dockerNetworkerr)
	}

	// creates a temp directory where postgres functionality are performed
	tempDatabaseDir, err_run := exec.Command("mktemp", "-d", "-t", "postgres-XXXXXXXXXX").Output()
	if err_run != nil {
		return EphemeralDB{}, fmt.Errorf("unable to mktemp: %v", err_run)
	}

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
			return false, fmt.Errorf("unable to docker run: %v", errDockerRun)
		}
		if dockerContainerID == nil {
			return false, fmt.Errorf("dockerContainerID is nil, after run")
		}
		// check for container status
		// #nosec G204
		status, _ := exec.Command("docker", "container", "inspect", "-f", "{{.State.Status}}", string(dockerContainerID)).Output()
		if string(status) == "running" {
			return true, nil
		}
		return true, nil
	})
	if errWait != nil {
		return EphemeralDB{}, fmt.Errorf("error in executing docker run command: %v", errWait)
	}

	// connects to the database inside the docker container
	errWait = wait.Poll(5*time.Second, 2*time.Minute, func() (bool, error) {
		// #nosec G204
		psqlcmd := exec.Command("docker", "exec", "--user", "postgres", "-e", "PGPASSWORD=gitops", "-i", dockerName, "psql", "-h", "localhost", "-d", tempDBName, "-U", "postgres", "-p", "5432", "-c", "select 1")
		var outb, errb bytes.Buffer
		psqlcmd.Stdout = &outb
		psqlcmd.Stderr = &errb

		psqlErr := psqlcmd.Run()

		if errb.String() != "" {
			return false, fmt.Errorf("error on psql run: %v", errb.String())
		}
		if psqlErr != nil {
			return false, fmt.Errorf("psqlErr is non-nil: %v", psqlErr)
		}
		return true, nil
	})
	if errWait != nil {
		return EphemeralDB{}, fmt.Errorf("error in executing docker run command: %v", errWait)
	}

	// creating a new database inside the postgres container
	newDBName := "postgres"
	psqlcmd := exec.Command("docker", "exec", "--user", "postgres", "-e", "PGPASSWORD=gitops", "-i", dockerName, "psql", "-h", "localhost", "-d", newDBName, "-U", "postgres", "-p", "5432")

	var errConnection bytes.Buffer
	psqlcmd.Stderr = &errConnection
	psqlErr := psqlcmd.Run()
	if errConnection.String() != "" {
		return EphemeralDB{}, fmt.Errorf("connection error: %v", errConnection.String())
	}
	if psqlErr != nil {
		return EphemeralDB{}, fmt.Errorf("error in creation: \nCommand Error: %v\nDatabase Error: %v", psqlErr, errConnection.String())
	}

	// Following command is used to populate the database tables from the db-schema.sql (defined in the monorepo)
	schemaToPostgresCont := exec.Command("docker", "cp", "../../../db-schema.sql", dockerName+":/")
	schemaToPostgresContErr := schemaToPostgresCont.Run()

	if schemaToPostgresContErr != nil {
		return EphemeralDB{}, fmt.Errorf("unable to execute docker cp of schema: %v", schemaToPostgresContErr)
	}
	psqlcmd = exec.Command("docker", "exec", "--user", "postgres", "-e", "PGPASSWORD=gitops", "-i", dockerName, "psql", "-h", "localhost", "-d", newDBName, "-U", "postgres", "-p", "5432", "-q", "-f", "db-schema.sql")
	var errSchema bytes.Buffer
	psqlcmd.Stderr = &errSchema

	psqlErr = psqlcmd.Run()

	if errSchema.String() != "" {
		return EphemeralDB{}, fmt.Errorf("errSchema is non-nil: %v", errSchema.String())
	}
	if psqlErr != nil {
		return EphemeralDB{}, fmt.Errorf("psql error: %v", psqlErr)
	}

	// connect the go code with the database
	database, err := ConnectToDatabaseWithPort(true, newDBName, 6432)
	if err != nil {
		return EphemeralDB{}, fmt.Errorf("unable to connect to database: %v", err)
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
func (ephemeralDB *EphemeralDB) Dispose() error {

	// To get the output of the command
	// #nosec G204
	_, err := exec.Command("docker", "rm", "-f", ephemeralDB.dbContainerID).Output()
	if err != nil {
		return fmt.Errorf("unable to docker rm -f container: %v", err)
	}

	// To get the output of the command
	// #nosec G204
	_, err = exec.Command("docker", "network", "rm", ephemeralDB.dockerNetwork).Output()
	if err != nil {
		return fmt.Errorf("unable to docker network rm: %v", err)
	}
	return nil
}
