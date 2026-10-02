// Copyright (c) 2026 haitch
// Licensed under the Apache License, Version 2.0
// https://www.apache.org/licenses/LICENSE-2.0

package oqlclassify

import "testing"

func TestClassify_BareSelectStar(t *testing.T) {
	got, err := Classify("SELECT * FROM companies")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !got.IsSimpleSelect {
		t.Fatalf("got %+v, want IsSimpleSelect=true", got)
	}
	if got.SourceTable != "companies" {
		t.Errorf("SourceTable = %q, want companies", got.SourceTable)
	}
}

func TestClassify_WhereClauseDoesNotDisqualify(t *testing.T) {
	got, err := Classify("SELECT * FROM companies WHERE industry = 'finance'")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !got.IsSimpleSelect || got.SourceTable != "companies" {
		t.Fatalf("got %+v, want a simple select on companies -- WHERE narrows rows, it doesn't change what each row is", got)
	}
}

func TestClassify_OrderByDoesNotDisqualify(t *testing.T) {
	got, err := Classify("SELECT * FROM companies ORDER BY name DESC")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !got.IsSimpleSelect {
		t.Fatalf("got %+v, want IsSimpleSelect=true -- ORDER BY only orders rows", got)
	}
}

func TestClassify_TopDoesNotDisqualify(t *testing.T) {
	got, err := Classify("SELECT TOP 10 * FROM companies")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !got.IsSimpleSelect {
		t.Fatalf("got %+v, want IsSimpleSelect=true -- TOP only limits row count", got)
	}
}

func TestClassify_OffsetFetchDoesNotDisqualify(t *testing.T) {
	got, err := Classify("SELECT * FROM companies ORDER BY name OFFSET 10 ROWS FETCH NEXT 5 ROWS ONLY")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !got.IsSimpleSelect {
		t.Fatalf("got %+v, want IsSimpleSelect=true -- OFFSET/FETCH only paginate rows", got)
	}
}

func TestClassify_ExplicitFieldListIsNotSimple(t *testing.T) {
	got, err := Classify("SELECT name, industry FROM companies")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.IsSimpleSelect {
		t.Fatalf("got %+v, want IsSimpleSelect=false -- an explicit field list is not SELECT *, even if it happens to name every field", got)
	}
}

func TestClassify_JoinIsNotSimple(t *testing.T) {
	got, err := Classify("SELECT * FROM companies c JOIN deals d ON c.id = d.company_id")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.IsSimpleSelect {
		t.Fatalf("got %+v, want IsSimpleSelect=false for a JOIN", got)
	}
}

func TestClassify_LeftJoinIsNotSimple(t *testing.T) {
	got, err := Classify("SELECT * FROM companies c LEFT JOIN deals d ON c.id = d.company_id")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.IsSimpleSelect {
		t.Fatalf("got %+v, want IsSimpleSelect=false for a LEFT JOIN", got)
	}
}

func TestClassify_AggregateIsNotSimple(t *testing.T) {
	got, err := Classify("SELECT COUNT(*) FROM companies")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.IsSimpleSelect {
		t.Fatalf("got %+v, want IsSimpleSelect=false for COUNT(*)", got)
	}
}

func TestClassify_MixedStarAndAggregateIsNotSimple(t *testing.T) {
	got, err := Classify("SELECT *, COUNT(*) FROM companies")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.IsSimpleSelect {
		t.Fatalf("got %+v, want IsSimpleSelect=false -- two columns, not a bare SELECT *", got)
	}
}

func TestClassify_GroupByIsNotSimple(t *testing.T) {
	got, err := Classify("SELECT * FROM companies GROUP BY industry")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.IsSimpleSelect {
		t.Fatalf("got %+v, want IsSimpleSelect=false -- tsqlparser accepts this syntactically even though it's semantically unusual, confirmed directly", got)
	}
}

func TestClassify_UnionIsNotSimple(t *testing.T) {
	got, err := Classify("SELECT * FROM companies UNION SELECT * FROM contacts")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.IsSimpleSelect {
		t.Fatalf("got %+v, want IsSimpleSelect=false for a UNION", got)
	}
}

func TestClassify_DistinctIsNotSimple(t *testing.T) {
	got, err := Classify("SELECT DISTINCT * FROM companies")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.IsSimpleSelect {
		t.Fatalf("got %+v, want IsSimpleSelect=false for DISTINCT", got)
	}
}

func TestClassify_SelectIntoIsNotSimple(t *testing.T) {
	got, err := Classify("SELECT * INTO backup_companies FROM companies")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.IsSimpleSelect {
		t.Fatalf("got %+v, want IsSimpleSelect=false for SELECT INTO -- this is a write operation, not a plain read", got)
	}
}

func TestClassify_SubqueryInFromIsNotSimple(t *testing.T) {
	got, err := Classify("SELECT * FROM (SELECT * FROM companies) sub")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.IsSimpleSelect {
		t.Fatalf("got %+v, want IsSimpleSelect=false -- a derived table is not a bare TableName", got)
	}
}

func TestClassify_MalformedQueryReturnsErrorNotPanic(t *testing.T) {
	got, err := Classify("SELECT * FROM FROM FROM")
	if err == nil {
		t.Fatalf("expected a parse error, got none (result: %+v)", got)
	}
	if got.IsSimpleSelect {
		t.Fatalf("got %+v, want IsSimpleSelect=false alongside the error", got)
	}
}

func TestClassify_NonSelectStatementIsNotSimple(t *testing.T) {
	got, err := Classify("UPDATE companies SET name = 'x'")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.IsSimpleSelect {
		t.Fatalf("got %+v, want IsSimpleSelect=false for a non-SELECT statement", got)
	}
}

func TestClassify_EmptyQueryIsNotSimple(t *testing.T) {
	got, err := Classify("")
	if got.IsSimpleSelect {
		t.Fatalf("got %+v (err=%v), want IsSimpleSelect=false for an empty query", got, err)
	}
}
