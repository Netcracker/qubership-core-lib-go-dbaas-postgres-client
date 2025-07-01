package pgdbaas

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/netcracker/qubership-core-lib-go/v3/configloader"
	dbaasbase "github.com/netcracker/qubership-core-lib-go-dbaas-base-client/v3"
	"github.com/netcracker/qubership-core-lib-go-dbaas-base-client/v3/model"
	. "github.com/netcracker/qubership-core-lib-go-dbaas-base-client/v3/testutils"
	params "github.com/netcracker/qubership-core-lib-go-dbaas-postgres-client/v4/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
	"github.com/uptrace/bun"
)

const (
	dbaasAgentUrlEnvName     = "dbaas.agent"
	namespaceEnvName         = "microservice.namespace"
	testServiceName          = "service_test"
	createDatabaseV3         = "/api/v3/dbaas/test_namespace/databases"
	getDatabaseV3            = "/api/v3/dbaas/test_namespace/databases/get-by-classifier/postgresql"
	username                 = "service_test"
	password                 = "qwerty127"
)

type DatabaseTestSuite struct {
	suite.Suite
	database Database
}

func (suite *DatabaseTestSuite) SetupSuite() {
	StartMockServer()
	os.Setenv(dbaasAgentUrlEnvName, GetMockServerUrl())
	os.Setenv(namespaceEnvName, "test_namespace")
	os.Setenv(propMicroserviceName, testServiceName)

	yamlParams := configloader.YamlPropertySourceParams{ConfigFilePath: "testdata/application.yaml"}
	configloader.Init(configloader.BasePropertySources(yamlParams)...)
}

func (suite *DatabaseTestSuite) TearDownSuite() {
	os.Unsetenv(dbaasAgentUrlEnvName)
	os.Unsetenv(namespaceEnvName)
	os.Unsetenv(propMicroserviceName)
	StopMockServer()
}

func (suite *DatabaseTestSuite) BeforeTest(suiteName, testName string) {
	suite.T().Cleanup(ClearHandlers)
	dbaasPool := dbaasbase.NewDbaaSPool()
	client := NewClient(dbaasPool)
	suite.database = client.ServiceDatabase()
}

func (suite *DatabaseTestSuite) TestServiceDbaasPgClient_FindDbaaSPostgresqlConnection() {
	AddHandler(Contains(getDatabaseV3), defaultDbaasResponseHandler)

	ctx := context.Background()
	actualResponse, err := suite.database.FindConnectionProperties(ctx)
	assert.Nil(suite.T(), err)
	assert.Equal(suite.T(), password, actualResponse.Password)
	assert.Equal(suite.T(), username, actualResponse.Username)
}

func (suite *DatabaseTestSuite) TestServiceDbaasPgClient_FindDbaaSPostgresqlConnection_ConnectionNotFound() {
	yamlParams := configloader.YamlPropertySourceParams{ConfigFilePath: "testdata/application.yaml"}
	configloader.Init(configloader.BasePropertySources(yamlParams)...)
	AddHandler(Contains(getDatabaseV3), func(writer http.ResponseWriter, request *http.Request) {
		writer.WriteHeader(http.StatusNotFound)
	})
	ctx := context.Background()

	if _, err := suite.database.FindConnectionProperties(ctx); assert.Error(suite.T(), err) {
		assert.IsType(suite.T(), model.DbaaSCreateDbError{}, err)
		assert.Equal(suite.T(), 404, err.(model.DbaaSCreateDbError).HttpCode)
	}
}

func (suite *DatabaseTestSuite) TestServiceDbaasPgClient_GetDbaaSPostgresqlConnection() {
	AddHandler(Contains(createDatabaseV3), defaultDbaasResponseHandler)
	ctx := context.Background()

	actualResponse, err := suite.database.GetConnectionProperties(ctx)
	assert.Nil(suite.T(), err)
	assert.Equal(suite.T(), password, actualResponse.Password)
	assert.Equal(suite.T(), username, actualResponse.Username)
}

func (suite *DatabaseTestSuite) TestServiceDbaasPgClient_GetDbaaSPostgresqlConnection_ConnectionNotFound() {
	AddHandler(Contains(createDatabaseV3), func(writer http.ResponseWriter, request *http.Request) {
		writer.WriteHeader(http.StatusNotFound)
	})
	ctx := context.Background()

	if _, err := suite.database.GetConnectionProperties(ctx); assert.Error(suite.T(), err) {
		assert.IsType(suite.T(), model.DbaaSCreateDbError{}, err)
		assert.Equal(suite.T(), 404, err.(model.DbaaSCreateDbError).HttpCode)
	}
}

func (suite *DatabaseTestSuite) TestServiceDbaasPgClient_GetPgClient_WithoutOptions() {
	actualPgClient, err := suite.database.GetPgClient()
	assert.Nil(suite.T(), err)
	assert.NotNil(suite.T(), actualPgClient)
}

func (suite *DatabaseTestSuite) TestServiceDbaasPgClient_GetPgClient_WithSqlOptions() {
	sqlOpts := []stdlib.OptionOpenDB{
		stdlib.OptionBeforeConnect(func(ctx context.Context, connConfig *pgx.ConnConfig) error {
			return nil
		}),
	}
	actualPgClient, err := suite.database.GetPgClient(&params.PgOptions{SqlOptions: sqlOpts})
	assert.Nil(suite.T(), err)
	assert.NotNil(suite.T(), actualPgClient)
}

func (suite *DatabaseTestSuite) TestServiceDbaasPgClient_GetPgClient_WithBunOptions() {
	bunOptions := []bun.DBOption{
		bun.WithDiscardUnknownColumns(),
	}
	actualPgClient, err := suite.database.GetPgClient(&params.PgOptions{BunOptions: bunOptions})
	assert.Nil(suite.T(), err)
	assert.NotNil(suite.T(), actualPgClient)
}

func TestSuite(t *testing.T) {
	suite.Run(t, new(DatabaseTestSuite))
}

func (suite *DatabaseTestSuite) TestToPgConnProperties_SslModeRequire() {
	connProperties := map[string]interface{}{
		"url":          "postgresql://pg-cpq-common.cpq-postgresql:5432/dbname",
		"username":     "service_test",
		"password":     "qwerty127",
		"role":         "admin",
		"tls":          true,
		"tlsNotStrict": true,
	}

	result := toPgConnProperties(connProperties)

	expectedUrl := "postgresql://pg-cpq-common.cpq-postgresql:5432/dbname?sslmode=require"
	assert.Equal(suite.T(), expectedUrl, result.Url, "URL should contains sslmode=require")
	assert.Equal(suite.T(), "service_test", result.Username)
	assert.Equal(suite.T(), "qwerty127", result.Password)
	assert.Equal(suite.T(), "admin", result.Role)
}

func (suite *DatabaseTestSuite) TestToPgConnProperties_SslModeVerifyFull() {
	connProperties := map[string]interface{}{
		"url":      "postgresql://pg-cpq-common.cpq-postgresql:5432/dbname",
		"username": "service_test",
		"password": "qwerty127",
		"role":     "admin",
		"tls":      true,
	}

	result := toPgConnProperties(connProperties)

	expectedUrl := "postgresql://pg-cpq-common.cpq-postgresql:5432/dbname?sslmode=verify-full"
	assert.Equal(suite.T(), expectedUrl, result.Url, "URL should contains sslmode=verify-full")
	assert.Equal(suite.T(), "service_test", result.Username)
	assert.Equal(suite.T(), "qwerty127", result.Password)
	assert.Equal(suite.T(), "admin", result.Role)
}

func defaultDbaasResponseHandler(writer http.ResponseWriter, request *http.Request) {
	writer.WriteHeader(http.StatusOK)
	connectionProperties := map[string]interface{}{
		"password": "qwerty127",
		"url":      "postgresql://pg-cpq-common.cpq-postgresql:5432/name",
		"username": "service_test",
		"role":     "admin",
	}
	dbResponse := model.LogicalDb{
		Id:                   "123",
		ConnectionProperties: connectionProperties,
	}
	jsonResponse, _ := json.Marshal(dbResponse)
	writer.Write(jsonResponse)
}
