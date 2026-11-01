package db

import (
	"errors"

	"gorm.io/gorm"

	"github.com/uasneppy/fuku_robot/fuku/utils/metrics"
)

// registerQueryMetrics counts every GORM statement in fuku_db_queries_total,
// labelled by operation only.
func registerQueryMetrics(database *gorm.DB) error {
	count := func(operation string) func(*gorm.DB) {
		counter := metrics.DBQueries.WithLabelValues(operation)
		return func(*gorm.DB) { counter.Inc() }
	}
	cb := database.Callback()
	return errors.Join(
		cb.Query().After("gorm:query").Register("fuku:metrics_query", count("query")),
		cb.Create().After("gorm:create").Register("fuku:metrics_create", count("create")),
		cb.Update().After("gorm:update").Register("fuku:metrics_update", count("update")),
		cb.Delete().After("gorm:delete").Register("fuku:metrics_delete", count("delete")),
		cb.Row().After("gorm:row").Register("fuku:metrics_row", count("row")),
		cb.Raw().After("gorm:raw").Register("fuku:metrics_raw", count("raw")),
	)
}
