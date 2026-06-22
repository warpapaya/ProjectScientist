package lab

func testActor(userID string) ActorContext {
	return MustActorContext(ActorContextInput{
		UserID:            userID,
		DisplayName:       userID,
		AuthProvider:      "test",
		TenantMemberships: []TenantMembership{{TenantID: DefaultTenantID}},
		RequestID:         "req-" + userID,
		CorrelationID:     "corr-" + userID,
	})
}
