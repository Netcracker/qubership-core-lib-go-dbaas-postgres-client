package pgdbaas

import (
	"context"

	dbaasbase "github.com/netcracker/qubership-core-lib-go-dbaas-base-client/v3"
	"github.com/netcracker/qubership-core-lib-go-dbaas-base-client/v3/cache"
	"github.com/netcracker/qubership-core-lib-go-dbaas-postgres-client/v4/model"
	"github.com/netcracker/qubership-core-lib-go/v3/logging"
)

var logger logging.Logger

func init() {
	logger = logging.GetLogger("pgdbaas")
}

const (
	propMicroserviceName = "microservice.name"
)

func NewClient(pool *dbaasbase.DbaaSPool) *DbaaSPostgreSqlClient {
	localCache := &cache.DbaaSCache{
		LogicalDbCache: make(map[cache.Key]interface{}),
	}
	return &DbaaSPostgreSqlClient{
		pgClientCache: localCache,
		pool:          pool,
	}
}

type DbaaSPostgreSqlClient struct {
	pgClientCache *cache.DbaaSCache
	pool          *dbaasbase.DbaaSPool
}

func (d *DbaaSPostgreSqlClient) ServiceDatabase(params ...model.DbParams) Database {
	return &database{
		params:        d.buildServiceDbParams(params),
		dbaasPool:     d.pool,
		postgresCache: d.pgClientCache,
	}
}

func (d *DbaaSPostgreSqlClient) buildServiceDbParams(params []model.DbParams) model.DbParams {
	localParams := model.DbParams{}
	if params != nil {
		localParams = params[0]
	}
	if localParams.Classifier == nil {
		localParams.Classifier = ServiceClassifier
	}
	return localParams
}

func (d *DbaaSPostgreSqlClient) TenantDatabase(params ...model.DbParams) Database {
	return &database{
		params:        d.buildTenantDbParams(params),
		dbaasPool:     d.pool,
		postgresCache: d.pgClientCache,
	}
}

func (d *DbaaSPostgreSqlClient) buildTenantDbParams(params []model.DbParams) model.DbParams {
	localParams := model.DbParams{}
	if params != nil {
		localParams = params[0]
	}
	if localParams.Classifier == nil {
		localParams.Classifier = TenantClassifier
	}
	return localParams
}

func ServiceClassifier(ctx context.Context) map[string]interface{} {
	return dbaasbase.BaseServiceClassifier(ctx)
}

func TenantClassifier(ctx context.Context) map[string]interface{} {
	return dbaasbase.BaseTenantClassifier(ctx)
}
