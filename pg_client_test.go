package pgdbaas

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgproto3"
	"github.com/jackc/pgx/v5/stdlib"
	dbaasbase "github.com/netcracker/qubership-core-lib-go-dbaas-base-client/v3"
	"github.com/netcracker/qubership-core-lib-go-dbaas-base-client/v3/cache"
	. "github.com/netcracker/qubership-core-lib-go-dbaas-base-client/v3/model"
	"github.com/netcracker/qubership-core-lib-go-dbaas-base-client/v3/model/rest"
	. "github.com/netcracker/qubership-core-lib-go-dbaas-base-client/v3/testutils"
	"github.com/netcracker/qubership-core-lib-go-dbaas-postgres-client/v4/model"
	"github.com/netcracker/qubership-core-lib-go-dbaas-postgres-client/v4/testdata/migrations/correct"
	incorrect "github.com/netcracker/qubership-core-lib-go-dbaas-postgres-client/v4/testdata/migrations/incorrect"
	"github.com/netcracker/qubership-core-lib-go/v3/configloader"
	"github.com/netcracker/qubership-core-lib-go/v3/security"
	"github.com/netcracker/qubership-core-lib-go/v3/serviceloader"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	pgcontainer "github.com/testcontainers/testcontainers-go/modules/postgres"
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

	// Verify that a subsequent cached call returns the same underlying sql.DB (fresh cached pool remains)
	sqlDB1, err := pgClient.GetSqlDb(ctx)
	assert.Nil(suite.T(), err)
	sqlDB2, err := pgClient.GetSqlDb(ctx)
	assert.Nil(suite.T(), err)
	assert.Equal(suite.T(), sqlDB1, sqlDB2)
}

// concurrentDbaasClient simulates an initial bad logical DB and a subsequent good logical DB on GetConnection
type concurrentDbaasClient struct {
	firstLogical       *LogicalDb
	refreshed          *LogicalDb
	mu                 sync.Mutex
	getOrCreateCalls   int
	getConnectionCalls int
}

func (c *concurrentDbaasClient) GetOrCreateDb(ctx context.Context, _ string, _ map[string]interface{}, _ rest.BaseDbParams) (*LogicalDb, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.getOrCreateCalls++
	return c.firstLogical, nil
}

func (c *concurrentDbaasClient) GetConnection(ctx context.Context, _ string, _ map[string]interface{}, _ rest.BaseDbParams) (map[string]interface{}, error) {
	c.mu.Lock()
	c.getConnectionCalls++
	c.mu.Unlock()
	return c.refreshed.ConnectionProperties, nil
}

func (suite *DatabaseTestSuite) TestPgClient_ConcurrentRecovery() {
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

	// initial logical DB points to a blackhole address that will fail ping
	badAddr := "127.0.0.1:65001"
	firstLogical := new(LogicalDb)
	require.NoError(suite.T(), json.Unmarshal(pgDbaasResponseHandler(badAddr, "badpass"), firstLogical))

	// refreshed logical DB points to the working container
	refreshedLogical := new(LogicalDb)
	require.NoError(suite.T(), json.Unmarshal(pgDbaasResponseHandler(addr, testContainerDbPassword), refreshedLogical))

	mock := &concurrentDbaasClient{firstLogical: firstLogical, refreshed: refreshedLogical}

	params := model.DbParams{Classifier: ServiceClassifier, BaseDbParams: rest.BaseDbParams{Role: "admin"}}
	pgClient := pgClientImpl{
		params:          params,
		postgresqlCache: &cache.DbaaSCache{LogicalDbCache: make(map[cache.Key]interface{})},
		dbaasClient:     mock,
	}

	// spawn concurrent callers that will race to recreate the pool
	var wg sync.WaitGroup
	callers := 10
	results := make([]*sql.DB, callers)
	errs := make([]error, callers)
	for i := 0; i < callers; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			db, err := pgClient.GetSqlDb(ctx)
			results[i] = db
			errs[i] = err
		}(i)
	}
	wg.Wait()

	for i := 0; i < callers; i++ {
		require.NoError(suite.T(), errs[i])
		require.NotNil(suite.T(), results[i])
	}

	// all callers should receive the same cached *sql.DB
	for i := 1; i < callers; i++ {
		assert.Equal(suite.T(), results[0], results[i])
	}

	// recovery is single-flight: fresh properties are fetched once and shared by all racing callers
	assert.Equal(suite.T(), 1, mock.getConnectionCalls)
	// the stale cached logical db must not be re-read during recovery
	assert.Equal(suite.T(), 1, mock.getOrCreateCalls)
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
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	pg, err := pgcontainer.Run(ctx,
		"postgres:16-alpine",
		pgcontainer.WithDatabase(testContainerDb),
		pgcontainer.WithUsername(testContainerDbUser),
		pgcontainer.WithPassword(testContainerDbPassword),
		pgcontainer.BasicWaitStrategies(),
	)
	require.NoError(t, err)

	return pg
}

func (suite *DatabaseTestSuite) TestPgClient_ContextCancellationRecreatesPool() {
	// blackhole server that will hang any connection attempts
	blackholeListener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(suite.T(), err)
	defer blackholeListener.Close()
	go func() {
		for {
			conn, err := blackholeListener.Accept()
			if err != nil {
				return
			}
			go func(c net.Conn) {
				defer c.Close()
				select {} // block forever
			}(conn)
		}
	}()

	logicalDb := new(LogicalDb)
	require.NoError(suite.T(), json.Unmarshal(
		pgDbaasResponseHandler(blackholeListener.Addr().String(), "any"),
		logicalDb,
	))
	dbaasClient := &contextCancellationDbaasClient{logicalDb: logicalDb}

	params := model.DbParams{Classifier: ServiceClassifier, BaseDbParams: rest.BaseDbParams{Role: "admin"}}
	client := pgClientImpl{
		params:          params,
		postgresqlCache: &cache.DbaaSCache{LogicalDbCache: make(map[cache.Key]interface{})},
		dbaasClient:     dbaasClient,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	// start DB creation in separate routine to not block the main thread
	type result struct {
		db  *sql.DB
		err error
	}
	dbCreationResult := make(chan result, 1)
	go func() {
		db, err := client.GetSqlDb(ctx)
		dbCreationResult <- result{db: db, err: err}
	}()

	select {
	case res := <-dbCreationResult:
		require.Error(suite.T(), res.err)
		require.Contains(suite.T(), res.err.Error(), "context deadline exceeded")
		require.Nil(suite.T(), res.db)
		require.Equal(suite.T(), 1, dbaasClient.getOrCreateCalls)
		require.Equal(suite.T(), 1, dbaasClient.getConnectionCalls)
		require.ErrorIs(suite.T(), dbaasClient.recreationContextErr, context.DeadlineExceeded)
	case <-time.After(1 * time.Second):
		suite.T().Fatal("Pool recreation did not return")
	}
}

type contextCancellationDbaasClient struct {
	logicalDb            *LogicalDb
	getOrCreateCalls     int
	getConnectionCalls   int
	recreationContextErr error
}

func (c *contextCancellationDbaasClient) GetOrCreateDb(
	ctx context.Context,
	_ string,
	_ map[string]interface{},
	_ rest.BaseDbParams,
) (*LogicalDb, error) {
	c.getOrCreateCalls++
	if c.getOrCreateCalls == 1 {
		return c.logicalDb, nil
	}
	return nil, fmt.Errorf("unexpected repeated GetOrCreateDb call")
}

// pool recreation refreshes connection properties through GetConnection, so this is where the
// cancelled context has to surface
func (c *contextCancellationDbaasClient) GetConnection(
	ctx context.Context,
	_ string,
	_ map[string]interface{},
	_ rest.BaseDbParams,
) (map[string]interface{}, error) {
	c.getConnectionCalls++
	c.recreationContextErr = ctx.Err()
	return nil, c.recreationContextErr
}

// authRotationDbaasClient serves connection properties carrying the rotated password.
type authRotationDbaasClient struct {
	rotated            *LogicalDb
	getOrCreateCalls   int
	getConnectionCalls int
}

func (c *authRotationDbaasClient) GetOrCreateDb(context.Context, string, map[string]interface{}, rest.BaseDbParams) (*LogicalDb, error) {
	c.getOrCreateCalls++
	return nil, fmt.Errorf("unexpected GetOrCreateDb call")
}

func (c *authRotationDbaasClient) GetConnection(_ context.Context, _ string, _ map[string]interface{}, _ rest.BaseDbParams) (map[string]interface{}, error) {
	c.getConnectionCalls++
	return c.rotated.ConnectionProperties, nil
}

// A pool whose credential was rotated after the connection was established stays pingable but fails
// queries with SQLSTATE 28P01. A real container cannot reproduce that: a wrong password there fails
// the ping first and diverts into the pool-recreation branch, so the password-refresh branch of
// GetSqlDb is only reachable against a backend that answers the ping and rejects the query.
func (suite *DatabaseTestSuite) TestPgClient_GetSqlDb_RefreshesRotatedPassword() {
	ctx := context.Background()
	backend := startFakePgBackend(suite.T())
	defer backend.stop()

	rotated := new(LogicalDb)
	require.NoError(suite.T(), json.Unmarshal(pgDbaasResponseHandler(backend.addr(), testContainerDbPassword), rotated))
	dbaasClient := &authRotationDbaasClient{rotated: rotated}

	params := model.DbParams{Classifier: ServiceClassifier, BaseDbParams: rest.BaseDbParams{Role: "admin"}}
	pgClient := pgClientImpl{
		params:          params,
		postgresqlCache: &cache.DbaaSCache{LogicalDbCache: make(map[cache.Key]interface{})},
		dbaasClient:     dbaasClient,
	}

	// seed the cache with a pool built from the pre-rotation password
	stale := new(LogicalDb)
	require.NoError(suite.T(), json.Unmarshal(pgDbaasResponseHandler(backend.addr(), wrongPassword), stale))
	staleConfig, err := buildPgConfig(stale.ConnectionProperties, false)
	require.NoError(suite.T(), err)
	stalePool := stdlib.OpenDB(*staleConfig)
	setConnectionSettings(stalePool)

	discriminator := pgDiscriminator{Role: params.BaseDbParams.Role, RoReplica: params.RoReplica}
	key := cache.NewKeyWithDiscriminator(DB_TYPE, params.Classifier(ctx), &discriminator)
	pgClient.postgresqlCache.LogicalDbCache[key] = stalePool

	refreshed, err := pgClient.GetSqlDb(ctx)
	require.NoError(suite.T(), err)
	require.NotNil(suite.T(), refreshed)

	// the pool was rebuilt from freshly fetched properties, not reused
	assert.NotSame(suite.T(), stalePool, refreshed)
	assert.Equal(suite.T(), 1, dbaasClient.getConnectionCalls)
	assert.Equal(suite.T(), 0, dbaasClient.getOrCreateCalls)

	// the cache holds the replacement, so the next caller does not pick up the closed pool
	assert.Same(suite.T(), refreshed, pgClient.postgresqlCache.LogicalDbCache[key])
	assert.ErrorContains(suite.T(), stalePool.PingContext(ctx), "database is closed")

	// the replacement is usable once the backend stops rejecting
	backend.acceptQueries()
	assert.NoError(suite.T(), refreshed.PingContext(ctx))
}

// fakePgBackend is a minimal PostgreSQL server: it completes the startup handshake and answers
// pgconn's "-- ping" query, but rejects "SELECT 1" with SQLSTATE 28P01 until acceptQueries is called.
type fakePgBackend struct {
	listener net.Listener
	mu       sync.Mutex
	rejects  bool
}

func startFakePgBackend(t *testing.T) *fakePgBackend {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	backend := &fakePgBackend{listener: listener, rejects: true}
	go backend.acceptLoop()
	return backend
}

func (s *fakePgBackend) addr() string { return s.listener.Addr().String() }

func (s *fakePgBackend) stop() { s.listener.Close() }

func (s *fakePgBackend) acceptQueries() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.rejects = false
}

func (s *fakePgBackend) rejecting() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.rejects
}

func (s *fakePgBackend) acceptLoop() {
	for {
		conn, err := s.listener.Accept()
		if err != nil {
			return
		}
		go s.serve(conn)
	}
}

func (s *fakePgBackend) serve(conn net.Conn) {
	defer conn.Close()
	backend := pgproto3.NewBackend(conn, conn)

	// decline SSL and GSSAPI until the frontend falls back to a plain startup message
	for {
		startup, err := backend.ReceiveStartupMessage()
		if err != nil {
			return
		}
		if _, isStartup := startup.(*pgproto3.StartupMessage); isStartup {
			break
		}
		if _, err := conn.Write([]byte("N")); err != nil {
			return
		}
	}

	backend.Send(&pgproto3.AuthenticationOk{})
	backend.Send(&pgproto3.ParameterStatus{Name: "server_version", Value: "16.0"})
	backend.Send(&pgproto3.ParameterStatus{Name: "client_encoding", Value: "UTF8"})
	backend.Send(&pgproto3.BackendKeyData{ProcessID: 1, SecretKey: []byte{0, 0, 0, 1}})
	backend.Send(&pgproto3.ReadyForQuery{TxStatus: 'I'})
	if err := backend.Flush(); err != nil {
		return
	}

	for {
		msg, err := backend.Receive()
		if err != nil {
			return
		}
		query, isQuery := msg.(*pgproto3.Query)
		if !isQuery {
			return // Terminate, or anything else this fake does not implement
		}
		if strings.Contains(query.String, "SELECT 1") && s.rejecting() {
			backend.Send(&pgproto3.ErrorResponse{
				Severity: "ERROR",
				Code:     "28P01",
				Message:  `password authentication failed for user "postgres"`,
			})
		} else {
			backend.Send(&pgproto3.CommandComplete{CommandTag: []byte("SELECT 0")})
		}
		backend.Send(&pgproto3.ReadyForQuery{TxStatus: 'I'})
		if err := backend.Flush(); err != nil {
			return
		}
	}
}
