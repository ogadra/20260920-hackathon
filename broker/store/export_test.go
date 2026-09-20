package store

// SetRandHexFnForTest は contract_test から randHexFn を注入するための test-only helper。
// production build には含まれず、Repository interface に test seam を露出させない。
func SetRandHexFnForTest(r Repository, fn func() string) {
	switch v := r.(type) {
	case *DynamoRepository:
		v.randHexFn = fn
	case *FirestoreRepository:
		v.randHexFn = fn
	}
}
