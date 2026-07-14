package opend

import (
	"errors"
	"testing"

	"google.golang.org/protobuf/proto"

	"github.com/jftrade/jftrade-main/pkg/futu/codec"
	commonpb "github.com/jftrade/jftrade-main/pkg/futu/pb/common"
	getglobalstatepb "github.com/jftrade/jftrade-main/pkg/futu/pb/getglobalstate"
	qotgetusersecuritygrouppb "github.com/jftrade/jftrade-main/pkg/futu/pb/qotgetusersecuritygroup"
)

func TestReadHelpersPropagateClosedClientErrors(t *testing.T) {
	client := &Client{}
	ctx := t.Context()
	cases := []struct {
		name string
		call func() error
	}{
		{"global state", func() error { _, err := client.GetGlobalState(ctx); return err }},
		{"subscription info", func() error { _, err := client.GetSubInfo(ctx, false); return err }},
		{"search quote", func() error { _, err := client.GetSearchQuote(ctx, "AAPL", 1); return err }},
		{"watchlist groups", func() error {
			_, err := client.GetUserSecurityGroups(ctx, qotgetusersecuritygrouppb.GroupType_GroupType_All)
			return err
		}},
		{"watchlist members", func() error { _, err := client.GetUserSecurities(ctx, "Growth"); return err }},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			if err := test.call(); !errors.Is(err, ErrClosed) {
				t.Fatalf("error = %v, want ErrClosed", err)
			}
		})
	}
}

func TestGlobalStateNormalizesMissingPayloadAndProgramStatusLabels(t *testing.T) {
	client, _, ctx := clientWithServer(t, map[uint32]protoHandler{
		ProtoGetGlobalState: func(codec.Frame) (proto.Message, error) {
			return &getglobalstatepb.Response{RetType: new(int32(0))}, nil
		},
	})

	state, err := client.GetGlobalState(ctx)
	if err != nil || state == nil || state.ServerVer != 0 || state.ProgramStatus != nil {
		t.Fatalf("GetGlobalState missing payload = (%#v, %v)", state, err)
	}
	if got := programStatusLabel(nil); got != "Unavailable" {
		t.Fatalf("programStatusLabel(nil) = %q", got)
	}
	if got := programStatusLabel(&commonpb.ProgramStatus{Type: commonpb.ProgramStatusType_ProgramStatusType_Ready.Enum()}); got != "ProgramStatusType_Ready" {
		t.Fatalf("programStatusLabel without description = %q", got)
	}
}
