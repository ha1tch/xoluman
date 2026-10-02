// Copyright (c) 2026 haitch
// Licensed under the Apache License, Version 2.0
// https://www.apache.org/licenses/LICENSE-2.0

// Package oqlclassify determines whether an OQL query's result rows are,
// one-to-one, complete and unmodified documents of a single real entity
// type — the one case where routing the result into xoluman's existing,
// live-editable entity grid is genuinely safe. Every other query shape
// (a field list rather than *, a JOIN, an aggregate, GROUP BY, UNION,
// SELECT INTO, DISTINCT) can still be shown as a read-only table, but
// its own rows no longer correspond to a single, complete, real record
// that a PATCH could safely target — so it is never routed to the
// editable grid, only ever to a plain table view.
//
// Classification is done by parsing the query with tsqlparser (a real
// T-SQL AST, the same library xolu's own OQL planner already depends on
// for equivalent decisions — see xolu's pkg/oql/planner.go and its own
// PushAggregate/PushJoin/PushFull push-down logic) rather than by text
// matching. A regex or hand-rolled string check for "select-star,
// one table, nothing else" would get real OQL syntax wrong somewhere
// (comments, bracketed identifiers, whitespace, subqueries) — and a
// false positive here is the one failure mode this package exists to
// avoid, since it would route a result into a live-edit grid that
// isn't actually safe to edit.
package oqlclassify

import (
	"fmt"
	"strings"

	"github.com/ha1tch/tsqlparser"
	"github.com/ha1tch/tsqlparser/ast"
)

// Result is what Classify reports about one OQL query.
type Result struct {
	// IsSimpleSelect is true only for SELECT * FROM <one table>, with
	// no JOIN, no aggregate, no GROUP BY, no UNION, no DISTINCT, no
	// SELECT INTO, no HAVING. WHERE, ORDER BY, TOP, OFFSET, and FETCH
	// never disqualify a query — they narrow or order which rows come
	// back, or how many, but never change what each returned row is:
	// still a complete, real entity document.
	IsSimpleSelect bool
	// SourceTable is the entity type a simple-select query targets —
	// only meaningful when IsSimpleSelect is true.
	SourceTable string
}

// Classify parses query and reports whether it is a simple select. A
// query that fails to parse, or that parses to anything other than a
// single SELECT statement, is reported as IsSimpleSelect: false — never
// an error on its own account, since the caller's own real concern is
// just "is this safe to route to the editable grid," and a query this
// package can't even parse is honestly answered "no" to that question,
// not a reason to fail the whole request. err is non-nil only for a
// genuine parse failure, so callers can still choose to surface it
// (e.g. as a hint the query itself may be invalid) without needing to
// treat it as blocking.
func Classify(query string) (Result, error) {
	program, errs := tsqlparser.Parse(query)
	if len(errs) > 0 {
		return Result{}, fmt.Errorf("oqlclassify: parse error: %s", strings.Join(errs, "; "))
	}
	if program == nil || len(program.Statements) != 1 {
		return Result{}, nil
	}
	stmt, ok := program.Statements[0].(*ast.SelectStatement)
	if !ok {
		return Result{}, nil
	}
	return classifySelect(stmt), nil
}

func classifySelect(stmt *ast.SelectStatement) Result {
	if stmt.Distinct {
		return Result{}
	}
	if stmt.Into != nil {
		return Result{}
	}
	if stmt.Union != nil {
		return Result{}
	}
	if len(stmt.GroupBy) > 0 {
		return Result{}
	}
	if stmt.Having != nil {
		return Result{}
	}
	if len(stmt.Columns) != 1 || !stmt.Columns[0].AllColumns {
		return Result{}
	}
	if stmt.From == nil || len(stmt.From.Tables) != 1 {
		return Result{}
	}
	tableName, ok := stmt.From.Tables[0].(*ast.TableName)
	if !ok {
		// A JoinClause, DerivedTable (subquery), or any other
		// TableReference variant also has len(Tables) == 1 by its own
		// construction (a join wraps both sides internally, it isn't
		// two separate slice entries) — confirmed directly against
		// tsqlparser's own ast.go, not assumed. Only a bare TableName
		// counts as "one real table" here.
		return Result{}
	}
	return Result{IsSimpleSelect: true, SourceTable: tableNameString(tableName)}
}

func tableNameString(t *ast.TableName) string {
	if t.Name == nil {
		return ""
	}
	parts := make([]string, len(t.Name.Parts))
	for i, p := range t.Name.Parts {
		parts[i] = p.Value
	}
	return strings.Join(parts, ".")
}
