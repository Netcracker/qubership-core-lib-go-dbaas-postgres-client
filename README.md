[![Go build](https://github.com/Netcracker/qubership-core-lib-go-dbaas-postgres-client/actions/workflows/go-build.yml/badge.svg)](https://github.com/Netcracker/qubership-core-lib-go-dbaas-postgres-client/actions/workflows/go-build.yml)
[![Coverage](https://sonarcloud.io/api/project_badges/measure?metric=coverage&project=Netcracker_qubership-core-lib-go-dbaas-postgres-client)](https://sonarcloud.io/summary/overall?id=Netcracker_qubership-core-lib-go-dbaas-postgres-client)
[![duplicated_lines_density](https://sonarcloud.io/api/project_badges/measure?metric=duplicated_lines_density&project=Netcracker_qubership-core-lib-go-dbaas-postgres-client)](https://sonarcloud.io/summary/overall?id=Netcracker_qubership-core-lib-go-dbaas-postgres-client)
[![vulnerabilities](https://sonarcloud.io/api/project_badges/measure?metric=vulnerabilities&project=Netcracker_qubership-core-lib-go-dbaas-postgres-client)](https://sonarcloud.io/summary/overall?id=Netcracker_qubership-core-lib-go-dbaas-postgres-client)
[![bugs](https://sonarcloud.io/api/project_badges/measure?metric=bugs&project=Netcracker_qubership-core-lib-go-dbaas-postgres-client)](https://sonarcloud.io/summary/overall?id=Netcracker_qubership-core-lib-go-dbaas-postgres-client)
[![code_smells](https://sonarcloud.io/api/project_badges/measure?metric=code_smells&project=Netcracker_qubership-core-lib-go-dbaas-postgres-client)](https://sonarcloud.io/summary/overall?id=Netcracker_qubership-core-lib-go-dbaas-postgres-client)

# Postgres dbaas go client

This module provides convenient way of interaction with **postgresql** databases provided by dbaas-aggregator.
`Postgresql dbaas go client` supports _multi-tenancy_ and can work with both _service_ and _tenant_ databases.

This module is based on [uptrace/bun](https://github.com/uptrace/bun) ORM library and [pgx](https://github.com/jackc/pgx) driver.

> **NOTE** If you want to migrate your service from go-microservice-core to new postgres-client please check our 
> [migration guide](/docs/pg-client-migration-guide.md)

> **NOTE** from version v1.0.0-beta.1 this library is working with uptrace/bun instead of go-pg library. 
> If you used previous versions, please check our [migration guide](/docs/bun-migration-guide.md) 

- [Install](#install)
- [Usage](#usage)
    * [Get connection for existing database or create new one](#get-connection-for-existing-database-or-create-new-one)
    * [Find connection for existing database](#find-connection-for-existing-database)
    * [PgClient](#pgclient)
    * [How to get pgx.Conn](#how-to-get-pgxconn)
- [Classifier](#classifier) 
- [Configuring connections](#configuring-connections)
- [Migrations during database creation](#migrations-during-database-creation)
    * [SQL migrations](#sql-migrations)
    * [Go migrations](#go-migrations)
    * [Process of migrations execution](#process-of-migrations-execution)
    * [Migrations example](#migrations-example)
- [SSL/TLS support](#ssltls-support)
- [Quick example](#quick-example)

## Install

To get `postgres dbaas client` use
```go
 go get github.com/netcracker/qubership-core-lib-go-dbaas-postgres-client@<latest released version>
```

List of all released versions may be found [here](https://github.com/netcracker/qubership-core-lib-go-dbaas-postgres-client/-/tags)

## Usage
At first, it's necessary to register security implemention - dummy or your own, the followning example shows registration of required services:
```go
import (
	"github.com/netcracker/qubership-core-lib-go/v3/serviceloader"
	"github.com/netcracker/qubership-core-lib-go/v3/security"
)

func init() {
	serviceloader.Register(1, &security.DummyToken{})
}
```

Then the user should create `DbaaSPostgreSqlClient`. This is a base client, which allows working with tenant and service databases.
To create instance of `DbaaSPostgreSqlClient` use `NewClient(pool *dbaasbase.DbaaSPool) *DbaaSPostgreSqlClient`. 

Note that client has parameter _pool_. `dbaasbase.DbaaSPool` is a tool which stores all cached connections and
create new ones. To find more info visit [dbaasbase](https://github.com/netcracker/qubership-core-lib-go-dbaas-base-client/blob/main/README.md)

Example of client creation:
```go
pool := dbaasbase.NewDbaasPool()
client := pgdbaas.NewClient(pool)
```

By default, Postgresql dbaas go client supports dbaas-aggregator as databases source. But there is a possibility for user to provide another
sources (for example, zookeeper). To do so use [LogcalDbProvider](https://github.com/netcracker/qubership-core-lib-go-dbaas-base-client/blob/main/README.md#logicaldbproviders) 
from dbaasbase.

Next step is to create `Database` object. `Databse` is not a pg.DB instance. It just an interface which allows 
creating pgClient or getting connection properties from dbaas. At this step user may choose which type of database he will 
work with: `service` or `tenant`.  

* To work with service databases use `ServiceDatabase(params ...model.DbParams) Database`
* To work with tenant databases use `TenantDatabase(params ...model.DbParams) Database`

Each func has `DbParams` as parameter. 

DbParams store information for database creation. Note that this parameter is optional, but if user doesn't pass Classifier,
default one will be used. More about classifiers [here](#classifier)

| Name         | Description                                                                                        | type                                                                                                                     |
|--------------|----------------------------------------------------------------------------------------------------|--------------------------------------------------------------------------------------------------------------------------|
| Classifier   | function which builds classifier from context. Classifier should be unique for each postgresql db. | func(ctx context.Context) map[string]interface{}                                                                         |
| BaseDbParams | Specific parameters for database creation.                                                         | [BaseDbParams](https://github.com/netcracker/qubership-core-lib-go-dbaas-base-client/blob/main#basedbparams) | 
| RoReplica    | Parameter for connection to readOnly replica. False by default.                                    | bool                                                                                                                     | 

Example how to create an instance of Database.
```go
 dbPool := dbaasbase.NewDbaasPool()
 client := pgdbaas.NewClient(dbPool)
 serviceDB := client.ServiceDatabase() // service Database creation 
 tenantDB := client.TenantDatabase() // tenant client creation 
```

`Database` allows: create new database and get connection to it, get connection to existing database and create pgClient. `serviceDB` and `tenantDB` should be singleton and 
it's enough to create them only once.

### Get connection for existing database or create new one

Func `GetConnectionProperties(ctx context.Context) (*model.PgConnProperties, error)`
at first will check if the desired database with _postgres_ type and classifier exists. If it exists, function will just return 
connection properties in the form of [PgConnProperties](model/pg_conn_properties.go).
If database with _postgres_ type and classifier doesn't exist, such database will be created and function will return 
connection properties for a new created database.

_Parameters:_
* ctx - context, enriched with some headers (See docs about context-propagation [here](https://github.com/netcracker/qubership-core-lib-go/blob/main/context-propagation/README.md)). Context object can have request scope values from which can be used to build classifier, for example tenantId.

```go
    ctx := ctxmanager.InitContext(context.Background(), propagateHeaders()) // preferred way
    // ctx := context.Background() // also possible for service client, but not recommended
    dbPgConnection, err := database.GetConnectionProperties(ctx)
```

### Find connection for existing database

Func `FindConnectionProperties(ctx context.Context) (*model.PgConnProperties, error)`
returns connection properties in the form of [PgConnProperties](model/pg_conn_properties.go). Unlike `GetConnectionProperties`
this function won't create database if it doesn't exist and just return nil value.

_Parameters:_
* ctx - context, enriched with some headers. (See docs about context-propagation [here](https://github.com/netcracker/qubership-core-lib-go/blob/main/context-propagation/README.md)). Context object can have request scope values from which can be used to build classifier, for example tenantId.

```go
    ctx := ctxmanager.InitContext(context.Background(), propagateHeaders()) // preferred way
    // ctx := context.Background() // also possible for service client, but not recommended
    dbPgConnection, err := database.FindConnectionProperties(ctx)
```

### PgClient

PgClient is a special object, which allows getting `sql.DB` or `bun.DB` to establish connection and to operate with a database. `PgClient` is a singleton and should be created only once.  

PgClient has two methods:
* `GetSqlDb(ctx context.Context) (*sql.DB, error)` which will return `*sql.DB`  
* `GetBunDb(ctx context.Context) (*bun.DB, error)` which will return `*bun.DB`

We strongly recommend not to store any of these objects as singleton and get new connection for every block of code. 
This is because the password in the database may change and then the connection will return an error. Every time the function
`pgClient.GetXXX()`is called, the password lifetime and correctness is checked. If necessary, the password is updated.  
`GetSqlDb` and `GetBunDb` functions allow to get `conn` object which is needed to make queries to database.  
Pay attention that you must close `conn` object after usage (see quick example) 

Note that: classifier will be created with context and function from DbParams. 

To create pgClient use `GetPgClient(options ...*model.PgOptions) (*pgClient, error)`

Parameters:
* options *model.PgOptions _optional_ - user may pass some desired configuration for bun.DB or for sql.DB or don't pass anything at all. 
model.PgOptions contains such fields as:
  - BunOptions []bun.DBOption - Options for bun.DB object creation
  - SqlOptions []stdlib.OptionOpenDB - Options for sql.DB object creation


```go
    ctx := ctxmanager.InitContext(context.Background(), propagateHeaders()) // preferred way
    // ctx := context.Background() // also possible for service client, but not recommended
    sqlOpts := []stdlib.OptionOpenDB{
      stdlib.OptionBeforeConnect(func(ctx context.Context, connConfig *pgx.ConnConfig) error {
		  logger.InfoC(ctx, "Before connect callback")
            return nil
      }),
    }
    pgClient, err := database.GetPgClient(&model.PgOptions{SqlOptions: sqlOpts}) // with options
    connection, err := pgClient.GetConnection(ctx)
```

### How to get pgx.Conn

User can acquire raw sql.DB and use it as base for any other library. Here is a convenient way to get
`pgx.Conn` from sql.DB

```go
    sqlDB, err := d.client.GetSqlDb(context.Background())
	if err != nil {
		logger.Error("Error during getting sql.DB: %+v", err.Error())
		return nil, err
	}

	conn, err := sqlDB.Conn(ctx)
	err = conn.Raw(func(driverConn interface{}) error {
		pgxConn := driverConn.(*stdlib.Conn).Conn() // this is *pgx.Conn
		// do some logic here
		return nil
	})
	if err != nil {
		return nil, err
	}
```

## Classifier

Classifier and dbType should be unique combination for each database.  Fields "tenantId" or "scope" must be into users' custom classifiers.

User can use default service or tenant classifier. It will be used if user doesn't specify Classifier in DbParams. This is recommended approach and and we don't recommend using custom classifier
because it can lead to some problems. Use can be reasonable if you migrate to this module and before used custom and not default classifier. 

Default service classifier looks like:
```json
{
    "scope": "service",
    "microserviceName": "<ms-name>",
    "namespace" : "<ms-namespace>"
}
```

Default tenant classifier looks like:
```json
{
  "scope" : "tenant",
  "tenantId": "<tenant-external-id>",
  "microserviceName": "<ms-name>",
  "namespace" : "<ms-namespace>"
}
```
Note, that if user doesn't set `MICROSERVICE_NAME` (or `microservice.name`) property, there will be panic during default classifier creation.
Also, if there are no tenantId in tenantContext, **panic will be thrown**.

## Configuring connections

| Parameter                       | Description                                                         | Type                                    | Default | Since        |
|---------------------------------|---------------------------------------------------------------------|-----------------------------------------|---------|--------------|
| dbaas.max.idle.connections      | Number of max possible open connections                             | int                                     | 5       | 1.0.0-beta.1 |
| dbaas.max.open.connections      | Number of max possible idle connections                             | int                                     | 5       | 1.0.0-beta.1 |
| dbaas.connections.max.lifetime  | Maximum amount of time a connection may be reused                   | int (will be multiplied to time.Second) | 60      | 1.0.0-beta.1 |
| dbaas.connections.max.idletime  | Maximum amount of time a connection may be idle before being closed | int (will be multiplied to time.Second) | 60      | 1.0.0-beta.1 |


## Migrations during database creation

Since 1.1.0

This library allow users to configure migrations which will be executed during database creation. This may be useful during
tenant databases creation, because they are creating on-hock. Migrations are based on `uptrace/bun`. Information about executed migrations will be stored in 
`bun_migrations` table. 

Two types of migrations are supported:
* sql migrations
* go migrations

Each type may contain information about up and down migrations. 

Migration naming rules: each migration should have name "timestamp"_"migration_comment"."type". For example "00000000000001_init.go".
We recommend using migration order instead of timestamp in order to store natural order of migrations.

### SQL migrations

* To register up sql migration create migration with name "some_unique_timestamp"_"migration_comment".up.sql
* To register down sql migrations create migration with name "some_unique_timestamp"_"migration_comment".down.sql

"some_unique_timestamp" should be equal for up and down migrations.

To register sql migrations do:
```go
var migrations = migrate.NewMigrations() // one variable per package with migrations

//go:embed *.sql
var sqlMigrations embed.FS

func GetMigrations() (*migrate.Migrations, error) {
    if err := migrations.Discover(sqlMigrations); err != nil {
		return nil, err
    }
	return migrations, nil
}
```

### Go migrations

Both up and down migrations should be stored in one file in function `MustRegister(func(ctx context.Context, db *bun.DB) error, func(ctx context.Context, db *bun.DB) error)`.
Note that this migration won't be executed in one transaction. In order to do it transactional wrap it with method 
`db.RunInTx(ctx context.Context, opts *sql.TxOptions, fn func(ctx context.Context, tx Tx) error)`.

```go
var migrations = migrate.NewMigrations() // one variable per package with migrations

func init() {
	migrations.MustRegister(func(ctx context.Context, db *bun.DB) error { // register migration in migrations
		// this function corresponds to UP migration
		return nil
	}, func(ctx context.Context, db *bun.DB) error {
        // this function corresponds to DOWN migration
		return nil
	})
}
```

### Process of migrations execution

1. Library calls `migrate.Init` method in order to check if migrations table (`bun_migrations`) already exists. If such table doesn't exist this method will create it. 
2. Library will try to execute received UP migrations
3. If there were any errors during migration procedure, library will stop executing migrations and will start rollback procedure
4. During rollback, it will try to execute existing DOWN migrations

### Migrations example

There should be one variable per package to register all necessary migrations (both go and sql)
```go
var migrations = migrate.NewMigrations()
```

Most convenient way is to create helper function which will return collected migrations
```go
var migrations = migrate.NewMigrations() // one variable per package with migrations

//go:embed *.sql
var sqlMigrations embed.FS // in order to find all *.sql files

func GetMigrations() (*migrate.Migrations, error) {
    if err := migrations.Discover(sqlMigrations); err != nil {
		return nil, err
    }
	return migrations, nil
}
```

User should pass collected migrations within struct [model.DbParams{}](model/db_params.go) with field `Migrations *migrate.Migrations`
during creation of `service` or `tenant` databases. User may have different migrations for each type of database.

```go
dbPool := dbaasbase.NewDbaaSPool()
pgDbaasClient := pgdbaas.NewClient(dbPool)
migrations, err := migration.GetMigrations()
// possible error handling
params := model.DbParams{Migrations: migrations}
servDB := pgDbaasClient.ServiceDatabase(params) // these migrations will be executed during first call to service database via pgClient GetBunDb or GetSqlDb methods
client, err := servDB.GetPgClient()

// or for tenant databases
tenantDB := pgDbaasClient.TenantDatabase(params) // these migrations will be executed during first call to tenant database via pgClient GetBunDb or GetSqlDb methods
client, err := tenantDB.GetPgClient()
```

## SSL/TLS support

This library supports work with secured connections to postgresql. Connection will be secured if TLS mode is enabled in 
postgresql-adapter.

For correct work with secured connections, the library requires having a truststore with certificate.
It may be public cloud certificate, cert-manager's certificate or any type of certificates related to database.
We do not recommend use self-signed certificates. Instead, use default NC-CA.

To start using TLS feature user has to enable it on the physical database (adapter's) side and add certificate to service truststore.

### Physical database switching

To enable TLS support in physical database redeploy postgresql with mandatory parameters
```yaml
tls.enabled=true;
```

In case of using cert-manager as certificates source add extra parameters
```yaml
ISSUER_NAME=<cluster issuer name>;
tls.certificateSecretName=pg-cert
tls.generateCerts.enabled=true
tls.generateCerts.clusterIssuerName=<cluster issuer name>
```

ClusterIssuerName identifies which Certificate Authority cert-manager will use to issue a certificate.
It can be obtained from the person in charge of the cert-manager on the environment.

### Add certificate to service truststore

The platform deployer provides the bulk uploading of certificates to truststores.

In order to add required certificates to services truststore:
1. Check and get certificate which is used in postgresql.
   * In most cases certificate is located in `Secrets` -> `pg-cert` -> `ca.crt`
2. Create ticket to `PSUPCDO/Configuration` and ask DevOps team to add this certificate to your deployer job.
3. After that all new deployments via configured deployer will include new certificate. Deployer creates a secret with certificate.
   Make sure the certificate is mount into your microservice
   On bootstrapping microservice there is generated truststore with default location and password. 


## Quick example

Here we create postgres tenant client, then get pgClient and execute a query for table creation.

application.yaml
```yaml
application.yaml
  
  microservice.name=tenant-manager
```

```go
package main

import (
	"context"
	"github.com/netcracker/qubership-core-lib-go/v3/configloader"
	"github.com/netcracker/qubership-core-lib-go/v3/context-propagation/ctxmanager"
  "github.com/netcracker/qubership-core-lib-go/v3/context-propagation/baseproviders/tenant"
	"github.com/netcracker/qubership-core-lib-go/v3/logging"
	dbaasbase "github.com/netcracker/qubership-core-lib-go-dbaas-base-client/v3"
	"github.com/netcracker/qubership-core-lib-go-dbaas-base-client/v3/model/rest"
	pgdbaas "github.com/netcracker/qubership-core-lib-go-dbaas-postgres-client/v4"
	"github.com/go-pg/pg/v10"
)

var logger logging.Logger

func init() {
	configloader.Init(configloader.BasePropertySources())
	logger = logging.GetLogger("main")
	ctxmanager.Register([]ctxmanager.ContextProvider{tenant.TenantProvider{}})
}

type Book struct {
  Code int
}

func main() { 
	// some context initialization 
	ctx := ctxmanager.InitContext(context.Background(), map[string]interface{}{tenant.TenantContextName: "id"})

    dbPool := dbaasbase.NewDbaaSPool()
    pgDbClient := pgdbaas.NewClient(dbPool)
    db := pgDbClient.TenantDatabase()
    client, _ := db.GetPgClient()  // singleton for tenant db. This object must be used to get connection in the entire application.
	
    db, err := client.GetBunDb(ctx) // now we can get bun.DB and work with SQL queries
    conn, err := db.Conn(ctx)
    defer conn.Close()              // connection must be closed after usage
    if err != nil {
        logger.Panicf("Got error during connection creation %+v", err)
    }
    res, errCreate := conn.NewCreateTable().Model((*Book)(nil)).Table(booksTable).IfNotExists().Exec(ctx)
    if errCreate != nil {
      logger.Panicf("Got error during table creation %+v", errCreate)
    }
	logger.Infof("Got result after create table script %+v", res)
    resInsert, err := addBook(&client, ctx)
    if err != nil {
      logger.Panicf("Got error during connection creation %+v", resInsert)
    }
    logger.Infof("Got result after insert into table script %+v", res)
}

func addBook(client *PgClient, ctx context.Context) (*sql.Result, error) {
    db, err := client.GetBunDB(ctx)
    if err != nil {
        return nil, err
    }
    conn, err := db.Conn(ctx)
    if err != nil {
        return nil, err
    }
    defer conn.Close()
    bookToInsert := Book{Code: 111}
    return conn.NewInsert().Model(&bookToInsert).Exec(ctx)
}

```

