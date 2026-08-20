package sys

import (
	"strings"

	"github.com/toothdy/cool-admin-go-next/cool/rest/crud"
)

func pageOrderBy(request crud.QueryRequest, columns map[string]string, defaultField string, defaultOrder string) (string, error) {
	terms, err := crud.ResolveSortTerms(request, columns, defaultField, defaultOrder)
	if err != nil {
		return "", err
	}
	parts := make([]string, 0, len(terms))
	for _, term := range terms {
		parts = append(parts, term.Column+" "+term.Order)
	}
	return " ORDER BY " + strings.Join(parts, ", "), nil
}

func pageLimit(request crud.QueryRequest) (string, []interface{}) {
	request = crud.NormalizePageRequest(request)
	if request.IsExport && request.MaxExportLimit > 0 {
		return " LIMIT ?", []interface{}{request.MaxExportLimit}
	}
	return " LIMIT ?, ?", []interface{}{(request.Page - 1) * request.Size, request.Size}
}

func sqlPageLimit(request crud.QueryRequest) (string, []interface{}) {
	request = crud.NormalizePageRequest(request)
	if request.IsExport {
		if request.MaxExportLimit > 0 {
			return " LIMIT ?", []interface{}{request.MaxExportLimit}
		}
		return " LIMIT ?", []interface{}{crud.MaxExportSize}
	}
	return " LIMIT ?, ?", []interface{}{(request.Page - 1) * request.Size, request.Size}
}
