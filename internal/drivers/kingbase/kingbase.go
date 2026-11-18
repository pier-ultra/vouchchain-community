package kingbase

import (
	"context"
	"fmt"

	"github.com/gogf/gf/contrib/drivers/pgsql/v2"
	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/util/gutil"
)

func init() {
	if err := gdb.Register("kingbase", &Driver{}); err != nil {
		panic(err)
	}
}

// Driver 复用 GoFrame pgsql 驱动，只覆盖 TableFields：金仓 V008 的系统表是 SYS_CATALOG.SYS_*，
// 没有 pg_catalog.pg_*，而 pgsql 驱动的元数据查询硬编码了后者。
type Driver struct {
	*pgsql.Driver
}

func (d *Driver) New(core *gdb.Core, node *gdb.ConfigNode) (gdb.DB, error) {
	base, err := d.Driver.New(core, node)
	if err != nil {
		return nil, err
	}
	return &Driver{Driver: base.(*pgsql.Driver)}, nil
}

func (d *Driver) TableFields(ctx context.Context, table string, schema ...string) (map[string]*gdb.TableField, error) {
	_ = gutil.GetOrDefaultStr(d.GetSchema(), schema...)
	sql := fmt.Sprintf(`
SELECT
  a.attname AS field,
  t.typname AS type,
  a.attnotnull AS null,
  (CASE WHEN d.contype = 'p' THEN 'pri' WHEN d.contype = 'u' THEN 'uni' ELSE '' END) AS key,
  ic.column_default AS default_value,
  b.description AS comment,
  COALESCE(character_maximum_length, numeric_precision, -1) AS length,
  numeric_scale AS scale
FROM SYS_CATALOG.SYS_ATTRIBUTE a
LEFT JOIN SYS_CATALOG.SYS_CLASS c ON a.attrelid = c.oid
LEFT JOIN SYS_CATALOG.SYS_CONSTRAINT d ON d.conrelid = c.oid AND a.attnum = d.conkey[1]
LEFT JOIN SYS_CATALOG.SYS_DESCRIPTION b ON a.attrelid = b.objoid AND a.attnum = b.objsubid
LEFT JOIN SYS_CATALOG.SYS_TYPE t ON a.atttypid = t.oid
LEFT JOIN information_schema.columns ic ON ic.column_name = a.attname AND ic.table_name = c.relname
WHERE c.oid = '%s'::regclass AND a.attisdropped IS FALSE AND a.attnum > 0
ORDER BY a.attnum`, table)
	link, err := d.SlaveLink()
	if err != nil {
		return nil, err
	}
	result, err := d.DoSelect(ctx, link, sql)
	if err != nil {
		return nil, err
	}
	fields := make(map[string]*gdb.TableField)
	for i, m := range result {
		name := m["field"].String()
		fieldType := m["type"].String()
		if l := m["length"].Int(); l > 0 {
			fieldType = fmt.Sprintf("%s(%d)", fieldType, l)
		}
		fields[name] = &gdb.TableField{
			Index:   i,
			Name:    name,
			Type:    fieldType,
			Null:    !m["null"].Bool(),
			Key:     m["key"].String(),
			Default: m["default_value"].Val(),
			Comment: m["comment"].String(),
		}
	}
	return fields, nil
}
