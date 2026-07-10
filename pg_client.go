package pgdbaas

import (
	"context"
	"database/sql"
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/stdlib"
	dbaasbase "github.com/netcracker/qubership-core-lib-go-dbaas-base-client/v3"
	"github.com/netcracker/qubership-core-lib-go-dbaas-base-client/v3/cache"
	"github.com/netcracker/qubership-core-lib-go-dbaas-base-client/v3/model/rest"
	"github.com/netcracker/qubership-core-lib-go-dbaas-postgres-client/v4/model"
	"github.com/netcracker/qubership-core-lib-go/v3/configloader"
	"github.com/netcracker/qubership-core-lib-go/v3/utils"
	"github.com/uptrace/bun"
	"github.com/uptrace/bun/dialect/pgdialect"
	"github.com/uptrace/bun/migrate"
)

var (
	maxIdleConnectionsProperty     = "dbaas.max.idle.connections"
	maxOpenConnectionsProperty     = "dbaas.max.open.connections"
	connectionsMaxLifetimeProperty = "dbaas.connections.max.lifetime"
	connectionsMaxIdleTimeProperty = "dbaas.connections.max.idletime"
)

const DEFAULT_CONNECTIONS_NUMBER = 5
const DEFAULT_TIMEOUT_S = 60

type PgClient interface {
	GetSqlDb(ctx context.Context) (*sql.DB, error)
	GetBunDb(ctx context.Context) (*bun.DB, error)
}

// PgClientImpl stores cache of databases and params for databases creation and getting connections
type pgClientImpl struct {
	bunOptions      []bun.DBOption
	sqlOptions      []stdlib.OptionOpenDB
	dbaasClient     dbaasbase.DbaaSClient
	postgresqlCache *cache.DbaaSCache
	params          model.DbParams
}

func (p *pgClientImpl) GetSqlDb(ctx context.Context) (*sql.DB, error) {
	classifier := p.params.Classifier(ctx)
	discriminator := pgDiscriminator{Role: p.params.BaseDbParams.Role, RoReplica: p.params.RoReplica}
	key := cache.NewKeyWithDiscriminator(DB_TYPE, classifier, &discriminator)
	pgDb, err := p.getOrCreateDb(ctx, key, classifier)
	if err != nil {
		return nil, err
	}
	// check if valid
	if pErr := pgDb.PingContext(ctx); pErr != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return nil, ctxErr
		}
		logger.Warnf("connection ping failed with err: %v. Deleting conn from cache and recreating connection", pErr)
		p.postgresqlCache.Delete(key)
		pgDb.Close()
		pgDb, err = p.getOrCreateDb(ctx, key, classifier)
		if err != nil {
			return nil, err
		}
	}
	if valid, vErr := p.isPasswordValid(ctx, pgDb); !valid && vErr == nil {
		logger.Info("authentication error, try to get new password")
		connConfig, vErr := p.getPasswordAgain(ctx, classifier, p.params.BaseDbParams)
		if vErr != nil {
			return nil, vErr
		}
		pgDb.Close()
		pgDb = stdlib.OpenDB(*connConfig, p.sqlOptions...)
		setConnectionSettings(pgDb)
		logger.Info("db password updated successfully")
	} else if vErr != nil {
		return nil, vErr
	}
	return pgDb, nil
}

func (p *pgClientImpl) getOrCreateDb(ctx context.Context, key cache.Key, classifier map[string]interface{}) (*sql.DB, error) {
	rawPgDb, err := p.postgresqlCache.Cache(key, p.createNewPgDb(ctx, classifier))
	if err != nil {
		return nil, err
	}
	return rawPgDb.(*sql.DB), nil
}

func setConnectionSettings(pgDb *sql.DB) {
	maxIdleConns := configloader.GetOrDefault(maxIdleConnectionsProperty, DEFAULT_CONNECTIONS_NUMBER)
	pgDb.SetMaxIdleConns(checkAndConvertToIntType(maxIdleConns, DEFAULT_CONNECTIONS_NUMBER))
	maxOpenConns := configloader.GetOrDefault(maxOpenConnectionsProperty, DEFAULT_CONNECTIONS_NUMBER)
	pgDb.SetMaxOpenConns(checkAndConvertToIntType(maxOpenConns, DEFAULT_CONNECTIONS_NUMBER))
	maxOpenTime := configloader.GetOrDefault(connectionsMaxLifetimeProperty, DEFAULT_TIMEOUT_S)
	pgDb.SetConnMaxLifetime(time.Duration(checkAndConvertToIntType(maxOpenTime, DEFAULT_TIMEOUT_S)) * time.Second)
	maxIdleTime := configloader.GetOrDefault(connectionsMaxIdleTimeProperty, DEFAULT_TIMEOUT_S)
	pgDb.SetConnMaxIdleTime(time.Duration(checkAndConvertToIntType(maxIdleTime, DEFAULT_TIMEOUT_S)) * time.Second)
}

func (p *pgClientImpl) GetBunDb(ctx context.Context) (*bun.DB, error) {
	sqlDb, err := p.GetSqlDb(ctx)
	if err != nil {
		return nil, err
	}
	dbBun := bun.NewDB(sqlDb, pgdialect.New(), p.bunOptions...)
	return dbBun, err
}

func (p *pgClientImpl) createNewPgDb(ctx context.Context, classifier map[string]interface{}) func() (interface{}, error) {
	return func() (interface{}, error) {
		roReplica := p.params.RoReplica
		logger.Info("Create postgresql database with classifier %+v", classifier)
		logicalDb, err := p.dbaasClient.GetOrCreateDb(ctx, DB_TYPE, classifier, p.params.BaseDbParams)
		if err != nil {
			return nil, err
		}
		config, err := buildPgConfig(logicalDb.ConnectionProperties, roReplica)
		if err != nil {
			return nil, err
		}
		logger.Debug("Build go-pg client for database with classifier %+v and type %s", classifier, DB_TYPE)
		if tls, ok := logicalDb.ConnectionProperties["tls"].(bool); ok && tls {
			logger.Infof("Connection to postgresql database will be secured")
			config.TLSConfig = utils.GetTlsConfig()
			config.TLSConfig.ServerName = logicalDb.ConnectionProperties["host"].(string)

		}
		sqlDb := stdlib.OpenDB(*config, p.sqlOptions...)
		setConnectionSettings(sqlDb)
		err = p.executeMigrationsIfAny(ctx, sqlDb)
		if err != nil {
			logger.ErrorC(ctx, "Error during migrations process: %+v", err.Error())
			return nil, err
		}

		return sqlDb, nil
	}
}

func (p *pgClientImpl) isPasswordValid(ctx context.Context, pgDb *sql.DB) (bool, error) {
	if _, err := pgDb.ExecContext(ctx, "SELECT 1;"); err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) {
			pgErrCode := pgErr.Code // Code: the SQLSTATE code for the error
			return strings.Compare(pgErrCode, "28P01") != 0, nil
		}
		return false, err
	}
	return true, nil
}

func (p *pgClientImpl) getPasswordAgain(ctx context.Context, classifier map[string]interface{}, params rest.BaseDbParams) (*pgx.ConnConfig, error) {
	newConnection, dbErr := p.dbaasClient.GetConnection(ctx, DB_TYPE, classifier, params)
	roReplica := p.params.RoReplica
	if dbErr != nil {
		logger.ErrorC(ctx, "Can't update connection with dbaas")
		return nil, dbErr
	}
	updatedOptions, err := buildPgConfig(newConnection, roReplica)
	if err != nil {
		return nil, err
	}
	return updatedOptions, nil
}

func buildPgConfig(connProperties map[string]interface{}, roReplica bool) (*pgx.ConnConfig, error) {
	if roReplica {
		if connProperties["roHost"] != nil {
			connProperties["url"] = setHostname(connProperties["url"].(string),
				connProperties["host"].(string),
				connProperties["roHost"].(string))
		} else {
			return nil, errors.New("connectionProperties does not contains roHost field. roReplica disabled")
		}
	}
	connectionProperties := toPgConnProperties(connProperties)
	config, err := pgx.ParseConfig(connectionProperties.Url)
	if err != nil {
		return nil, err
	}
	config.User = connectionProperties.Username
	config.Password = connectionProperties.Password
	return config, nil
}

func (p *pgClientImpl) executeMigrationsIfAny(ctx context.Context, sqlDb *sql.DB) error {
	if p.params.Migrations == nil {
		logger.DebugC(ctx, "There are no migrations for execution before creation")
		return nil
	}
	dbBun := bun.NewDB(sqlDb, pgdialect.New(), p.bunOptions...)
	migrator := migrate.NewMigrator(dbBun, p.params.Migrations)
	err := migrator.Init(ctx)
	if err != nil {
		return err
	}
	group, err := migrator.Migrate(ctx)
	if err != nil {
		return performRollback(err, migrator, ctx)
	}
	if group.IsZero() {
		logger.Info("There are no new migrations to run (database is up to date)")
	} else {
		logger.Info("Migrated successfully for group %s", group)
	}
	return nil
}

func performRollback(err error, migrator *migrate.Migrator, ctx context.Context) error {
	logger.Errorf("Can't execute migrations because of error: %+v. Ready to rollback", err.Error())
	rollback, rollbackErr := migrator.Rollback(ctx)
	if rollbackErr != nil {
		logger.Errorf("Rollback failed because of error: %+v", rollbackErr.Error())
		return rollbackErr
	}
	logger.Infof("Rollback migrations for group %+v", rollback)
	return err
}

func checkAndConvertToIntType(value interface{}, defValue int) int {
	finalValue := defValue
	switch value.(type) {
	case int:
		finalValue = value.(int)
	case string:
		intValue, err := strconv.Atoi(value.(string))
		if err == nil {
			finalValue = intValue
		}
	}
	return finalValue
}

func setHostname(addr string, oldHost string, newHost string) string {
	newUrl := strings.Replace(addr, oldHost, newHost, -1)
	return newUrl
}
