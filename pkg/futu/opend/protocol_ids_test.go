package opend

import "testing"

func TestStaticInfoAndKLineUpdateProtocolIDsDoNotOverlap(t *testing.T) {
	if ProtoGetStaticInfo != 3202 {
		t.Fatalf("ProtoGetStaticInfo = %d, want 3202", ProtoGetStaticInfo)
	}
	if ProtoQotUpdateKL != 3007 {
		t.Fatalf("ProtoQotUpdateKL = %d, want 3007", ProtoQotUpdateKL)
	}
}
