package store

// SetRandHexFnForTest は contract_test から randHexFn を注入するための test-only helper。
// production build には含まれず、Repository interface に test seam を露出させない。
func SetRandHexFnForTest(r Repository, fn func() string) {
	r.(*DynamoRepository).randHexFn = fn
}
