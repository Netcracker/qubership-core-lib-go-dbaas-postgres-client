package pgdbaas

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/docker/go-connections/nat"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/netcracker/qubership-core-lib-go/v3/configloader"
	"github.com/netcracker/qubership-core-lib-go/v3/serviceloader"
	"github.com/netcracker/qubership-core-lib-go/v3/security"
	dbaasbase "github.com/netcracker/qubership-core-lib-go-dbaas-base-client/v3"
	"github.com/netcracker/qubership-core-lib-go-dbaas-base-client/v3/cache"
	. "github.com/netcracker/qubership-core-lib-go-dbaas-base-client/v3/model"
	"github.com/netcracker/qubership-core-lib-go-dbaas-base-client/v3/model/rest"
	. "github.com/netcracker/qubership-core-lib-go-dbaas-base-client/v3/testutils"
	"github.com/netcracker/qubership-core-lib-go-dbaas-postgres-client/v4/model"
	"github.com/netcracker/qubership-core-lib-go-dbaas-postgres-client/v4/testdata/migrations/correct"
	incorrect "github.com/netcracker/qubership-core-lib-go-dbaas-postgres-client/v4/testdata/migrations/incorrect"
	"github.com/stretchr/testify/assert"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
	"github.com/uptrace/bun"
)

const (
	postgresPort            = "5432"
	testContainerDbPassword = "123qwerty"
	testContainerHost       = "localhost"
	testContainerDbUser     = "postgres"
	testContainerDb         = "demo"
	wrongPassword           = "qwerty123"
)

func init() {
	serviceloader.Register(1, &security.DummyToken{})
}

// entity for database tests
type Book struct {
	Code int
}

func (suite *DatabaseTestSuite) TestPgClient_GetConnection_ConnectionError() {
	ctx := context.Background()

	AddHandler(Contains(createDatabaseV3), func(writer http.ResponseWriter, request *http.Request) {
		writer.WriteHeader(http.StatusOK)
		jsonString := pgDbaasResponseHandler("localhost:65000", testContainerDbPassword)
		writer.Write(jsonString)
	})

	params := model.DbParams{Classifier: ServiceClassifier, BaseDbParams: rest.BaseDbParams{Role: "admin"}}
	pgClient := pgClientImpl{
		params:          params,
		postgresqlCache: &cache.DbaaSCache{LogicalDbCache: make(map[cache.Key]interface{})},
		dbaasClient:     dbaasbase.NewDbaasClient(),
	}
	conn, err := pgClient.GetSqlDb(ctx)
	assert.Nil(suite.T(), conn)
	assert.NotNil(suite.T(), err)
}

func (suite *DatabaseTestSuite) TestPgClient_GetBunDb_NewClient() {
	ctx := context.Background()
	pgContainer := prepareTestContainer(suite.T(), ctx)
	defer func() {
		err := pgContainer.Terminate(ctx)
		if err != nil {
			suite.T().Fatal(err)
		}
	}()

	addr, err := pgContainer.Endpoint(ctx, "")
	if err != nil {
		suite.T().Error(err)
	}

	AddHandler(Contains(createDatabaseV3), func(writer http.ResponseWriter, request *http.Request) {
		writer.WriteHeader(http.StatusOK)
		jsonString := pgDbaasResponseHandler(addr, testContainerDbPassword)
		writer.Write(jsonString)
	})

	params := model.DbParams{Classifier: ServiceClassifier, BaseDbParams: rest.BaseDbParams{Role: "admin"}}
	pgClient := pgClientImpl{
		params:          params,
		postgresqlCache: &cache.DbaaSCache{LogicalDbCache: make(map[cache.Key]interface{})},
		dbaasClient:     dbaasbase.NewDbaasClient(),
	}
	dbBun, err := pgClient.GetBunDb(ctx)
	assert.Nil(suite.T(), err)
	assert.NotNil(suite.T(), dbBun)
	// check that connection allows storing and getting info from db
	suite.checkConnectionIsWorking(dbBun, ctx)
}

func (suite *DatabaseTestSuite) TestPgClient_GetBunDbWithRoReplica_NewClient() {
	ctx := context.Background()
	pgContainer := prepareTestContainer(suite.T(), ctx)
	defer func() {
		err := pgContainer.Terminate(ctx)
		if err != nil {
			suite.T().Fatal(err)
		}
	}()

	addr, err := pgContainer.Endpoint(ctx, "")
	if err != nil {
		suite.T().Error(err)
	}

	AddHandler(Contains(createDatabaseV3), func(writer http.ResponseWriter, request *http.Request) {
		writer.WriteHeader(http.StatusOK)
		jsonString := pgDbaasResponseHandler(addr, testContainerDbPassword)
		writer.Write(jsonString)
	})

	params := model.DbParams{Classifier: ServiceClassifier, BaseDbParams: rest.BaseDbParams{Role: "admin"}, RoReplica: true}
	pgClient := pgClientImpl{
		params:          params,
		postgresqlCache: &cache.DbaaSCache{LogicalDbCache: make(map[cache.Key]interface{})},
		dbaasClient:     dbaasbase.NewDbaasClient(),
	}
	dbBun, err := pgClient.GetBunDb(ctx)
	assert.Nil(suite.T(), err)
	assert.NotNil(suite.T(), dbBun)
}

func (suite *DatabaseTestSuite) TestPgClient_GetBunDb_ClientFromCache() {
	ctx := context.Background()
	pgContainer := prepareTestContainer(suite.T(), ctx)
	defer func() {
		err := pgContainer.Terminate(ctx)
		if err != nil {
			suite.T().Fatal(err)
		}
	}()

	addr, err := pgContainer.Endpoint(ctx, "")
	if err != nil {
		suite.T().Error(err)
	}

	counter := 0
	AddHandler(Contains(createDatabaseV3), func(writer http.ResponseWriter, request *http.Request) {
		writer.WriteHeader(http.StatusOK)
		jsonString := pgDbaasResponseHandler(addr, testContainerDbPassword)
		writer.Write(jsonString)
		counter++
	})

	params := model.DbParams{Classifier: ServiceClassifier, BaseDbParams: rest.BaseDbParams{Role: "admin"}}
	pgClient := pgClientImpl{
		params:          params,
		postgresqlCache: &cache.DbaaSCache{LogicalDbCache: make(map[cache.Key]interface{})},
		dbaasClient:     dbaasbase.NewDbaasClient(),
	}
	firstConn, err := pgClient.GetBunDb(ctx)
	assert.Nil(suite.T(), err)
	assert.NotNil(suite.T(), firstConn)
	assert.Equal(suite.T(), 1, counter)

	// check that connection allows storing and getting info from db
	suite.checkConnectionIsWorking(firstConn, ctx)

	secondConn, err := pgClient.GetBunDb(ctx)
	assert.Nil(suite.T(), err)
	assert.NotNil(suite.T(), secondConn)
	assert.Equal(suite.T(), 1, counter)

	// check that connection allows storing and getting info from db
	suite.checkConnectionIsWorking(secondConn, ctx)
}

func (suite *DatabaseTestSuite) TestPgClient_GetBunDb_UpdatePassword() {
	ctx := context.Background()
	pgContainer := prepareTestContainer(suite.T(), ctx)
	defer func() {
		err := pgContainer.Terminate(ctx)
		if err != nil {
			suite.T().Fatal(err)
		}
	}()
	addr, err := pgContainer.Endpoint(ctx, "")
	if err != nil {
		suite.T().Error(err)
	}

	// create database with wrong password
	AddHandler(matches(createDatabaseV3), func(writer http.ResponseWriter, request *http.Request) {
		writer.WriteHeader(http.StatusOK)
		jsonString := pgDbaasResponseHandler(addr, wrongPassword)
		writer.Write(jsonString)
	})
	// update right password
	AddHandler(matches(getDatabaseV3), func(writer http.ResponseWriter, request *http.Request) {
		writer.WriteHeader(http.StatusOK)
		jsonString := pgDbaasResponseHandler(addr, testContainerDbPassword)
		writer.Write(jsonString)
	})
	params := model.DbParams{Classifier: ServiceClassifier, BaseDbParams: rest.BaseDbParams{Role: "admin"}}
	pgClient := pgClientImpl{
		params:          params,
		postgresqlCache: &cache.DbaaSCache{LogicalDbCache: make(map[cache.Key]interface{})},
		dbaasClient:     dbaasbase.NewDbaasClient(),
	}

	conn, err := pgClient.GetBunDb(ctx)
	assert.Nil(suite.T(), err)
	assert.NotNil(suite.T(), conn)

	// check that connection allows storing and getting info from db
	suite.checkConnectionIsWorking(conn, ctx)
}

func (suite *DatabaseTestSuite) TestPgClient_GetConnection_RawSqlDb() {
	ctx := context.Background()
	pgContainer := prepareTestContainer(suite.T(), ctx)
	defer func() {
		err := pgContainer.Terminate(ctx)
		if err != nil {
			suite.T().Fatal(err)
		}
	}()

	addr, err := pgContainer.Endpoint(ctx, "")
	if err != nil {
		suite.T().Error(err)
	}

	AddHandler(Contains(createDatabaseV3), func(writer http.ResponseWriter, request *http.Request) {
		writer.WriteHeader(http.StatusOK)
		jsonString := pgDbaasResponseHandler(addr, testContainerDbPassword)
		writer.Write(jsonString)
	})

	params := model.DbParams{Classifier: ServiceClassifier, BaseDbParams: rest.BaseDbParams{Role: "admin"}}
	pgClient := pgClientImpl{
		params:          params,
		postgresqlCache: &cache.DbaaSCache{LogicalDbCache: make(map[cache.Key]interface{})},
		dbaasClient:     dbaasbase.NewDbaasClient(),
	}
	sqlDB, err := pgClient.GetSqlDb(ctx)
	assert.Nil(suite.T(), err)
	assert.NotNil(suite.T(), sqlDB)
	// check that connection allows storing and getting info from db
	pingErr := sqlDB.Ping()
	assert.Nil(suite.T(), pingErr)
}

func (suite *DatabaseTestSuite) TestPgClient_GetConnection_PgxConn() {
	ctx := context.Background()
	pgContainer := prepareTestContainer(suite.T(), ctx)
	defer func() {
		err := pgContainer.Terminate(ctx)
		if err != nil {
			suite.T().Fatal(err)
		}
	}()

	addr, err := pgContainer.Endpoint(ctx, "")
	if err != nil {
		suite.T().Error(err)
	}

	AddHandler(Contains(createDatabaseV3), func(writer http.ResponseWriter, request *http.Request) {
		writer.WriteHeader(http.StatusOK)
		jsonString := pgDbaasResponseHandler(addr, testContainerDbPassword)
		writer.Write(jsonString)
	})

	params := model.DbParams{Classifier: ServiceClassifier, BaseDbParams: rest.BaseDbParams{Role: "admin"}}
	pgClient := pgClientImpl{
		params:          params,
		postgresqlCache: &cache.DbaaSCache{LogicalDbCache: make(map[cache.Key]interface{})},
		dbaasClient:     dbaasbase.NewDbaasClient(),
	}
	sqlDB, err := pgClient.GetSqlDb(ctx)
	assert.Nil(suite.T(), err)
	assert.NotNil(suite.T(), sqlDB)
	// check that connection allows storing and getting info from db
	conn, err := sqlDB.Conn(ctx)
	assert.Nil(suite.T(), err)
	conn.Raw(func(driverConn interface{}) error {
		pgxConn := driverConn.(*stdlib.Conn).Conn() // conn is a *pgx.Conn
		pingErr := pgxConn.Ping(ctx)
		assert.Nil(suite.T(), pingErr)
		return nil
	})
}

func (suite *DatabaseTestSuite) TestPgClient_GetBunDb_CheckMigrations() {
	ctx := context.Background()
	pgContainer := prepareTestContainer(suite.T(), ctx)
	defer func() {
		err := pgContainer.Terminate(ctx)
		if err != nil {
			suite.T().Fatal(err)
		}
	}()

	addr, err := pgContainer.Endpoint(ctx, "")
	if err != nil {
		suite.T().Error(err)
	}

	AddHandler(Contains(createDatabaseV3), func(writer http.ResponseWriter, request *http.Request) {
		writer.WriteHeader(http.StatusOK)
		jsonString := pgDbaasResponseHandler(addr, testContainerDbPassword)
		writer.Write(jsonString)
	})

	migrations, err := correct.GetCorrectMigrations()
	assert.Nil(suite.T(), err)

	params := model.DbParams{
		Classifier:   ServiceClassifier,
		BaseDbParams: rest.BaseDbParams{Role: "admin"},
		Migrations:   migrations,
	}
	pgClient := pgClientImpl{
		params:          params,
		postgresqlCache: &cache.DbaaSCache{LogicalDbCache: make(map[cache.Key]interface{})},
		dbaasClient:     dbaasbase.NewDbaasClient(),
	}
	dbBun, err := pgClient.GetBunDb(ctx)
	assert.Nil(suite.T(), err)
	assert.NotNil(suite.T(), dbBun)

	bookForSelect := make([]Book, 0)
	errSelect := dbBun.NewSelect().Model(&bookForSelect).Scan(ctx)
	assert.Nil(suite.T(), errSelect)
	assert.Equal(suite.T(), 1, len(bookForSelect))
	assert.Equal(suite.T(), 111, bookForSelect[0].Code)

	_, errDrop := dbBun.NewDropTable().Model((*Book)(nil)).Exec(ctx)
	assert.Nil(suite.T(), errDrop)
	_, errDrop = dbBun.NewDropTable().Table("bun_migrations").Exec(ctx)
	assert.Nil(suite.T(), errDrop)
}

func (suite *DatabaseTestSuite) TestPgClient_GetBunDb_CheckRollback() {
	ctx := context.Background()
	pgContainer := prepareTestContainer(suite.T(), ctx)
	defer func() {
		err := pgContainer.Terminate(ctx)
		if err != nil {
			suite.T().Fatal(err)
		}
	}()

	addr, err := pgContainer.Endpoint(ctx, "")
	if err != nil {
		suite.T().Error(err)
	}

	AddHandler(Contains(createDatabaseV3), func(writer http.ResponseWriter, request *http.Request) {
		writer.WriteHeader(http.StatusOK)
		jsonString := pgDbaasResponseHandler(addr, testContainerDbPassword)
		writer.Write(jsonString)
	})

	migrations, err := incorrect.GetIncorrectMigrations()
	assert.Nil(suite.T(), err)

	params := model.DbParams{
		Classifier:   ServiceClassifier,
		BaseDbParams: rest.BaseDbParams{Role: "admin"},
		Migrations:   migrations,
	}
	pgClient := pgClientImpl{
		params:          params,
		postgresqlCache: &cache.DbaaSCache{LogicalDbCache: make(map[cache.Key]interface{})},
		dbaasClient:     dbaasbase.NewDbaasClient(),
	}
	_, err = pgClient.GetBunDb(ctx)
	assert.NotNil(suite.T(), err)
}

func (suite *DatabaseTestSuite) checkConnectionIsWorking(conn *bun.DB, ctx context.Context) {
	booksTable := "books"
	_, errCreate := conn.NewCreateTable().Model((*Book)(nil)).Table(booksTable).IfNotExists().Exec(ctx)
	assert.Nil(suite.T(), errCreate)
	bookToInsert := Book{Code: 111}
	_, errInsert := conn.NewInsert().Model(&bookToInsert).Exec(ctx)
	assert.Nil(suite.T(), errInsert)
	bookForSelect := make([]Book, 0)
	errSelect := conn.NewSelect().Model(&bookForSelect).Scan(ctx)
	assert.Nil(suite.T(), errSelect)
	assert.Equal(suite.T(), 1, len(bookForSelect))
	assert.Equal(suite.T(), 111, bookForSelect[0].Code)
	_, errDrop := conn.NewDropTable().Model((*Book)(nil)).Table(booksTable).Exec(ctx)
	assert.Nil(suite.T(), errDrop)
}

func (suite *DatabaseTestSuite) TestPgClient_GetConnection_RawSqlDb_WithStringProperty() {
	os.Setenv("DBAAS_MAX_OPEN_CONNECTIONS", "2")
	configloader.Init(configloader.EnvPropertySource())

	ctx := context.Background()
	pgContainer := prepareTestContainer(suite.T(), ctx)
	defer func() {
		err := pgContainer.Terminate(ctx)
		if err != nil {
			suite.T().Fatal(err)
		}
	}()

	addr, err := pgContainer.Endpoint(ctx, "")
	if err != nil {
		suite.T().Error(err)
	}

	AddHandler(Contains(createDatabaseV3), func(writer http.ResponseWriter, request *http.Request) {
		writer.WriteHeader(http.StatusOK)
		jsonString := pgDbaasResponseHandler(addr, testContainerDbPassword)
		writer.Write(jsonString)
	})

	params := model.DbParams{Classifier: ServiceClassifier, BaseDbParams: rest.BaseDbParams{Role: "admin"}}
	pgClient := pgClientImpl{
		params:          params,
		postgresqlCache: &cache.DbaaSCache{LogicalDbCache: make(map[cache.Key]interface{})},
		dbaasClient:     dbaasbase.NewDbaasClient(),
	}
	sqlDB, err := pgClient.GetSqlDb(ctx)
	assert.Nil(suite.T(), err)
	assert.NotNil(suite.T(), sqlDB)
	// check that connection allows storing and getting info from db
	pingErr := sqlDB.Ping()
	assert.Nil(suite.T(), pingErr)
	assert.Equal(suite.T(), 2, sqlDB.Stats().MaxOpenConnections)
	defer func() { os.Unsetenv("DBAAS_MAX_OPEN_CONNECTIONS") }()
}

func (suite *DatabaseTestSuite) TestReconnectOnTcpTearDown() {
	ctx := context.Background()
	pgContainer := prepareTestContainer(suite.T(), ctx)
	defer func() {
		err := pgContainer.Terminate(ctx)
		if err != nil {
			suite.T().Fatal(err)
		}
	}()
	addr, err := pgContainer.Endpoint(ctx, "")
	if err != nil {
		suite.T().Error(err)
	}
	AddHandler(Contains(createDatabaseV3), func(writer http.ResponseWriter, request *http.Request) {
		writer.WriteHeader(http.StatusOK)
		jsonString := pgDbaasResponseHandler(addr, testContainerDbPassword)
		writer.Write(jsonString)
	})
	params := model.DbParams{Classifier: ServiceClassifier, BaseDbParams: rest.BaseDbParams{Role: "admin"}}
	pgClient := pgClientImpl{
		params:          params,
		postgresqlCache: &cache.DbaaSCache{LogicalDbCache: make(map[cache.Key]interface{})},
		dbaasClient:     dbaasbase.NewDbaasClient(),
	}
	conn, err := pgClient.GetSqlDb(ctx)
	assert.Nil(suite.T(), err)
	assert.NotNil(suite.T(), conn)
	//  drop tcp connections of the cached dbaas pg connection
	stopDuration := 5 * time.Second
	assert.Nil(suite.T(), pgContainer.Stop(ctx, &stopDuration))
	assert.Nil(suite.T(), pgContainer.Start(ctx))

	addr, err = pgContainer.Endpoint(ctx, "")
	if err != nil {
		suite.T().Error(err)
	}
	conn, err = pgClient.GetSqlDb(ctx)
	assert.Nil(suite.T(), err)
	assert.NotNil(suite.T(), conn)
}

func matches(submatch string) func(string) bool {
	return func(path string) bool {
		return strings.EqualFold(path, submatch)
	}
}

func pgDbaasResponseHandler(address, password string) []byte {
	url := fmt.Sprintf("postgresql://%s/%s", address, testContainerDb)
	splitAddr := strings.Split(address, ":")
	connectionProperties := map[string]interface{}{
		"password": password,
		"url":      url,
		"username": testContainerDbUser,
		"roHost":   splitAddr[0],
		"host":     splitAddr[0],
		"role":     "admin",
	}
	dbResponse := LogicalDb{
		Id:                   "123",
		ConnectionProperties: connectionProperties,
	}
	jsonResponse, _ := json.Marshal(dbResponse)
	return jsonResponse
}

func prepareTestContainer(t *testing.T, ctx context.Context) testcontainers.Container {
	os.Setenv("TESTCONTAINERS_RYUK_DISABLED", "true")

	env := map[string]string{
		"POSTGRES_USER":     testContainerDbUser,
		"POSTGRES_PASSWORD": testContainerDbPassword,
		"POSTGRES_DB":       testContainerDb,
	}
	port, _ := nat.NewPort("tcp", postgresPort)
	req := testcontainers.ContainerRequest{
		Image:        "postgres:15.6",
		ExposedPorts: []string{port.Port()},
		Env:          env,
		WaitingFor: wait.ForAll(
			wait.ForListeningPort(port).WithStartupTimeout(120 * time.Second),
		),
	}
	pgContainer, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: req,
		Started:          true,
	})
	if err != nil {
		t.Error(err)
	}

	os.Unsetenv("TESTCONTAINERS_RYUK_DISABLED")

	return pgContainer
}
