# Migration guide

This article contains essential information about migration process from _go-microservice-core_ library to new golang 
postgres-client library.

* [Updated classifier](#updated-classifier)
    - [Service classifier](#service-classifier)
    - [Tenant classifier](#tenant-classifier)

## Updated classifier

`go-microservice-core` provided incorrect classifier for both service and tenant databases. We fixed it in new libraries 
and now correct classifier is created by default. But as the classifier is used to obtain a connection to the database, 
using the default classifier may result in the creation of a new database and loss of information. We provide option, when 
user can set their own custom classifier, and we recommend using this functionality in such situation.

### Service classifier

In `go-microservice-core` service classifier looked like:
```json
{
  "isService" : "true",
  "namespace" : "<namespace>",
  "dbClassifier" : "default",
  "microserviceName" : "<name>"
}
```

In `postgres-client` service classifier now looks like:
```json
{
  "scope" : "service",
  "namespace" : "<namespace>",
  "microserviceName" : "<name>"
}
```

Example of function which will create classifier in old format and how to use this function:
```go
func createOldClassifier(ctx context.Context) map[string]interface{} {
	classifier := make(map[string]interface{})
	classifier["microserviceName"] = configloader.GetKoanf().MustString("microservice.name")
	classifier["isService"] = "true"
	classifier["dbClassifier"] = "default"
	classifier["namespace"] = configloader.GetKoanf().MustString("microservice.namespace")
	return classifier
}

func main() {
    dbPool := dbaasbase.NewDbaaSPool()
    pgDbClient := pgdbaas.NewClient(dbPool)
    
    dbParams := model.DbParams{
        Classifier: createOldClassifier,
    }
    pgClient, err := pgDbClient.ServiceDatabase(dbParams).GetPgClient()	
}
```

### Tenant classifier

In `go-microservice-core` tenant classifier looked like:
```json
{
  "tenantId": "<tenantId>",
  "namespace" : "<namespace>",
  "dbClassifier" : "default",
  "microserviceName" : "<name>"
}
```

In `postgres-client` tenant classifier now looks like:
```json
{
  "scope": "tenant",
  "tenantId": "<tenantId>",
  "namespace" : "<namespace>",
  "microserviceName" : "<name>"
}
```

Example of function which will create classifier in old format and how to use this function:
```go
func createOldClassifier(ctx context.Context) map[string]interface{} {
	classifier := make(map[string]interface{})
	classifier["microserviceName"] = configloader.GetKoanf().MustString("microservice.name")
	tenantObject, err := tenant.Of(ctx)
	if err != nil || tenantObject.GetTenant() == "" { 
		logger.PanicC(ctx, "Can't create tenant database, tenantId is absent") 
	}
	classifier["tenantId"] = tenantObject.GetTenant()
	classifier["dbClassifier"] = "default"
	classifier["namespace"] = configloader.GetKoanf().MustString("microservice.namespace")
	return classifier
}

func main() {
    dbPool := dbaasbase.NewDbaaSPool()
    pgDbClient := pgdbaas.NewClient(dbPool)
    
    dbParams := model.DbParams{
        Classifier: createOldClassifier,
    }
    pgClient, err := pgDbClient.TenantDatabase(dbParams).GetPgClient()	
}
```

