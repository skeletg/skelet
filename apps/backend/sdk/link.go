package sdk

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"reflect"
	"regexp"
	"strings"
	"time"
	"unicode"

	"github.com/google/uuid"
)

// A plugin table holding rows for core records, keyed by the record's id - what a field on a core type usually reads.
// LoadOne and LoadMany answer such a field in one query, so the plugin's resolver is a line:
//
//	var warranties = sdk.Link{Table: "acme_warranties", Key: "product_id"}
//
//	func (adminResolver) ProductAcmeWarranty(ctx context.Context, objs []*model.Product) ([]*model.AcmeWarranty, error) {
//	    if err := sdk.RequirePermission(ctx, "catalog.view"); err != nil {
//	        return nil, err
//	    }
//	    return sdk.LoadOne[model.AcmeWarranty](ctx, warranties, sdk.IDs(objs, func(p *model.Product) string { return p.ID }))
//	}
//
// The columns are the model's fields: each exported field with a json tag reads the column of that name in snake case
// (monthsLeft reads months_left). Fields of other kinds - a nested object, a list - are left for resolvers of their own
type Link struct {
	Table string // the plugin's table, prefixed with its slug
	Key   string // its column holding the core record's id, a uuid
	Order string // for LoadMany, the column a record's rows are ordered by - empty leaves the database's order
}

// Postgres identifiers as a plugin names its own tables and columns - they go into SQL unquoted
var linkIdent = regexp.MustCompile(`^[a-z_][a-z0-9_]{0,62}$`)

// IDs is each record's id, in the records' order - what LoadOne and LoadMany take. A nil record, which a list with
// holes in it hands a batch resolver, has the empty id, which loads nothing
func IDs[T any](objs []T, id func(T) string) []string {
	out := make([]string, len(objs))
	for i, o := range objs {
		if v := reflect.ValueOf(o); v.Kind() == reflect.Pointer && v.IsNil() {
			continue
		}
		out[i] = id(o)
	}
	return out
}

// LoadOne is one T per id, in the ids' order, nil where the table has no row - for a field holding one value
func LoadOne[T any](ctx context.Context, l Link, ids []string) ([]*T, error) {
	rows, err := loadLinked[T](ctx, l, ids)
	if err != nil {
		return nil, err
	}
	out := make([]*T, len(ids))
	for i, id := range ids {
		if found := rows[strings.ToLower(id)]; len(found) > 0 {
			out[i] = found[0]
		}
	}
	return out, nil
}

// LoadMany is every row per id, in the ids' order, empty where the table has none - for a list field
func LoadMany[T any](ctx context.Context, l Link, ids []string) ([][]*T, error) {
	rows, err := loadLinked[T](ctx, l, ids)
	if err != nil {
		return nil, err
	}
	out := make([][]*T, len(ids))
	for i, id := range ids {
		out[i] = rows[strings.ToLower(id)]
		if out[i] == nil {
			out[i] = []*T{}
		}
	}
	return out, nil
}

func loadLinked[T any](ctx context.Context, l Link, ids []string) (map[string][]*T, error) {
	for _, ident := range []string{l.Table, l.Key} {
		if !linkIdent.MatchString(ident) {
			return nil, fmt.Errorf("sdk.Link: %q is not a lowercase table or column name", ident)
		}
	}
	if l.Order != "" && !linkIdent.MatchString(l.Order) {
		return nil, fmt.Errorf("sdk.Link: %q is not a lowercase column name", l.Order)
	}
	cols, fields, err := linkColumns(reflect.TypeFor[T]())
	if err != nil {
		return nil, err
	}
	// Only well-formed ids reach the array literal - a malformed one matches nothing anyway
	var wanted []string
	for _, id := range ids {
		if u, err := uuid.Parse(id); err == nil {
			wanted = append(wanted, u.String())
		}
	}
	out := map[string][]*T{}
	if len(wanted) == 0 {
		return out, nil
	}
	db, err := Database()
	if err != nil {
		return nil, err
	}
	query := fmt.Sprintf(`SELECT %s::text, %s FROM %s WHERE %s = ANY($1::uuid[])`, l.Key, strings.Join(cols, ", "), l.Table, l.Key)
	if l.Order != "" {
		query += " ORDER BY " + l.Order
	}
	rows, err := db.QueryContext(ctx, query, "{"+strings.Join(wanted, ",")+"}")
	if err != nil {
		return nil, fmt.Errorf("sdk.Link %s: %w", l.Table, err)
	}
	defer rows.Close()
	for rows.Next() {
		var key string
		v := new(T)
		dest := []any{&key}
		rv := reflect.ValueOf(v).Elem()
		for _, f := range fields {
			dest = append(dest, rv.FieldByIndex(f).Addr().Interface())
		}
		if err := rows.Scan(dest...); err != nil {
			return nil, fmt.Errorf("sdk.Link %s: %w", l.Table, err)
		}
		out[key] = append(out[key], v)
	}
	return out, rows.Err()
}

// The columns T reads and the fields they land in - scalars and their pointers only, so a nested object or a list
// stays its own resolver's to fill
func linkColumns(t reflect.Type) ([]string, [][]int, error) {
	if t.Kind() != reflect.Struct {
		return nil, nil, fmt.Errorf("sdk.Link: %s is not a struct", t)
	}
	var cols []string
	var fields [][]int
	for f := range t.Fields() {
		name, _, _ := strings.Cut(f.Tag.Get("json"), ",")
		if !f.IsExported() || name == "" || name == "-" || !scannable(f.Type) {
			continue
		}
		cols = append(cols, snake(name))
		fields = append(fields, f.Index)
	}
	if len(cols) == 0 {
		return nil, nil, errors.New("sdk.Link: " + t.String() + " has no field a column can fill")
	}
	return cols, fields, nil
}

var (
	timeType    = reflect.TypeFor[time.Time]()
	scannerType = reflect.TypeFor[sql.Scanner]()
)

func scannable(t reflect.Type) bool {
	if t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	if t == timeType || reflect.PointerTo(t).Implements(scannerType) {
		return true
	}
	switch t.Kind() {
	case reflect.String, reflect.Bool, reflect.Int, reflect.Int32, reflect.Int64, reflect.Float32, reflect.Float64:
		return true
	}
	return false
}

// monthsLeft is months_left, and productID product_id
func snake(name string) string {
	var b strings.Builder
	r := []rune(name)
	for i, c := range r {
		if unicode.IsUpper(c) {
			if i > 0 && (unicode.IsLower(r[i-1]) || (i+1 < len(r) && unicode.IsLower(r[i+1]))) {
				b.WriteByte('_')
			}
			c = unicode.ToLower(c)
		}
		b.WriteRune(c)
	}
	return b.String()
}
