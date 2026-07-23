package opend

import "testing"

func TestStaticInfoAndKLineUpdateProtocolIDsDoNotOverlap(t *testing.T) {
	if ProtoGetUserInfo != 1005 {
		t.Fatalf("ProtoGetUserInfo = %d, want 1005", ProtoGetUserInfo)
	}
	if ProtoGetStaticInfo != 3202 {
		t.Fatalf("ProtoGetStaticInfo = %d, want 3202", ProtoGetStaticInfo)
	}
	if ProtoGetPlateSet != 3204 || ProtoGetPlateSecurity != 3205 {
		t.Fatalf("plate protocol IDs = %d/%d, want 3204/3205", ProtoGetPlateSet, ProtoGetPlateSecurity)
	}
	if ProtoQotUpdateKL != 3007 {
		t.Fatalf("ProtoQotUpdateKL = %d, want 3007", ProtoQotUpdateKL)
	}
	if ProtoGetSearchQuote != 3262 {
		t.Fatalf("ProtoGetSearchQuote = %d, want 3262", ProtoGetSearchQuote)
	}
	if ProtoGetSubInfo != 3003 {
		t.Fatalf("ProtoGetSubInfo = %d, want 3003", ProtoGetSubInfo)
	}
}
