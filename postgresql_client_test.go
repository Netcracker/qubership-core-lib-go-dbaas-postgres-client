package pgdbaas

import (
	"context"
	"os"
	"testing"

	"github.com/netcracker/qubership-core-lib-go/v3/context-propagation/baseproviders/tenant"
	"github.com/netcracker/qubership-core-lib-go/v3/configloader"
	"github.com/netcracker/qubership-core-lib-go/v3/serviceloader"
	"github.com/netcracker/qubership-core-lib-go/v3/security"
	"github.com/netcracker/qubership-core-lib-go/v3/context-propagation/ctxmanager"
	dbaasbase "github.com/netcracker/qubership-core-lib-go-dbaas-base-client/v3"
	"github.com/netcracker/qubership-core-lib-go-dbaas-base-client/v3/model/rest"
	"github.com/netcracker/qubership-core-lib-go-dbaas-postgres-client/v4/model"
	"github.com/stretchr/testify/assert"
)

func init() {
	ctxmanager.Register([]ctxmanager.ContextProvider{tenant.TenantProvider{}})
	serviceloader.Register(1, &security.DummyToken{})
}

func setup() {
	os.Setenv(propMicroserviceName, "test_service")
	os.Setenv(namespaceEnvName, "test_space")
	configloader.InitWithSourcesArray([]*configloader.PropertySource{configloader.EnvPropertySource()})
}

func tearDown() {
	os.Unsetenv(propMicroserviceName)
	os.Unsetenv(namespaceEnvName)
}

func TestNewServiceDbaasClient_WithoutParams(t *testing.T) {
	setup()
	defer tearDown()
	dbaasPool := dbaasbase.NewDbaaSPool()
	commonClient := NewClient(dbaasPool)
	serviceDB := commonClient.ServiceDatabase()
	assert.NotNil(t, serviceDB)
	db := serviceDB.(*database)
	ctx := context.Background()

	assert.Equal(t, ServiceClassifier(ctx), db.params.Classifier(ctx))
}

func TestNewServiceDbaasClient_WithParams(t *testing.T) {
	setup()
	defer tearDown()
	dbaasPool := dbaasbase.NewDbaaSPool()
	commonClient := NewClient(dbaasPool)
	params := model.DbParams{
		Classifier:   stubClassifier,
		BaseDbParams: rest.BaseDbParams{},
	}
	serviceDB := commonClient.ServiceDatabase(params)
	assert.NotNil(t, serviceDB)
	db := serviceDB.(*database)
	ctx := context.Background()
	assert.Equal(t, stubClassifier(ctx), db.params.Classifier(ctx))
}

func TestNewTenantDbaasClient_WithoutParams(t *testing.T) {
	setup()
	defer tearDown()
	dbaasPool := dbaasbase.NewDbaaSPool()
	commonClient := NewClient(dbaasPool)
	tenantDb := commonClient.TenantDatabase()
	assert.NotNil(t, tenantDb)
	db := tenantDb.(*database)
	ctx := createTenantContext()
	assert.Equal(t, TenantClassifier(ctx), db.params.Classifier(ctx))
}

func TestNewTenantDbaasClient_WithParams(t *testing.T) {
	setup()
	defer tearDown()
	dbaasPool := dbaasbase.NewDbaaSPool()
	commonClient := NewClient(dbaasPool)
	params := model.DbParams{
		Classifier:   stubClassifier,
		BaseDbParams: rest.BaseDbParams{},
	}
	tenantDb := commonClient.TenantDatabase(params)
	assert.NotNil(t, tenantDb)
	db := tenantDb.(*database)
	ctx := context.Background()
	assert.Equal(t, stubClassifier(ctx), db.params.Classifier(ctx))
}

func TestCreateServiceClassifier(t *testing.T) {
	setup()
	defer tearDown()
	expected := map[string]interface{}{
		"microserviceName": "test_service",
		"namespace":        "test_space",
		"scope":            "service",
	}
	actual := ServiceClassifier(context.Background())
	assert.Equal(t, expected, actual)
}

func TestCreateTenantClassifier(t *testing.T) {
	setup()
	defer tearDown()
	ctx := createTenantContext()
	expected := map[string]interface{}{
		"microserviceName": "test_service",
		"namespace":        "test_space",
		"tenantId":         "id",
		"scope":            "tenant",
	}
	actual := TenantClassifier(ctx)
	assert.Equal(t, expected, actual)
}

func TestCreateTenantClassifier_WithoutTenantId(t *testing.T) {
	setup()
	defer tearDown()
	ctx := context.Background()

	assert.Panics(t, func() {
		TenantClassifier(ctx)
	})
}

func TestBuildServiceDbParams(t *testing.T) {
	setup()
	defer tearDown()
	dbaasPool := dbaasbase.NewDbaaSPool()
	commonClient := NewClient(dbaasPool)
	params := model.DbParams{
		Classifier:   stubClassifier,
		BaseDbParams: rest.BaseDbParams{Role: "admin"},
	}
	serviceDbParams := commonClient.buildServiceDbParams([]model.DbParams{params})
	assert.NotNil(t, serviceDbParams)
	assert.Equal(t, serviceDbParams.BaseDbParams, params.BaseDbParams)
	ctx := context.Background()
	assert.Equal(t, stubClassifier(ctx), serviceDbParams.Classifier(ctx))

	params = model.DbParams{}
	serviceDbParams = commonClient.buildServiceDbParams([]model.DbParams{params})
	assert.NotNil(t, serviceDbParams)
	expected := map[string]interface{}{
		"microserviceName": "test_service",
		"namespace":        "test_space",
		"scope":            "service",
	}
	assert.Equal(t, expected, serviceDbParams.Classifier(ctx))
}

func stubClassifier(ctx context.Context) map[string]interface{} {
	return map[string]interface{}{
		"scope":            "service",
		"microserviceName": "service_test",
	}
}

func createTenantContext() context.Context {
	incomingHeaders := map[string]interface{}{tenant.TenantHeader: "id"}
	return ctxmanager.InitContext(context.Background(), incomingHeaders)
}
