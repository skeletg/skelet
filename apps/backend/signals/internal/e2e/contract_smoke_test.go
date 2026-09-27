package e2e

import (
	"context"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/reflect/protoregistry"
	"google.golang.org/protobuf/types/dynamicpb"

	"github.com/skeletg/skelet/apps/backend/core/lib/auth"
	"github.com/skeletg/skelet/apps/backend/core/signalsapi"
	_ "github.com/skeletg/skelet/apps/backend/core/signalsapi/v1"
)

// Every unary method on the contract, called once with an empty request against the graph as cmd/signals
// builds it. A handler may refuse the input or find nothing - what it may not do is fail inside, which is
// what an unwired dependency looks like from the dashboard: a button that answers "not configured"
func TestEveryContractMethodAnswersFromTheWiredGraph(t *testing.T) {
	h := newHarness(t)
	lis := bufconn.Listen(1 << 20)
	go func() { _ = h.d.Contract.Serve(lis) }()
	t.Cleanup(func() { h.d.Contract.Stop() })
	cc, err := grpc.NewClient("passthrough:///bufnet",
		grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) { return lis.DialContext(ctx) }),
		grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = cc.Close() })

	admin := tokenCtx(t, h.service, auth.ServiceClaims{AdminID: uuid.New(), SessionID: uuid.New(), IsSuperuser: true})
	system := tokenCtx(t, h.service, auth.ServiceClaims{System: true})
	broken := map[codes.Code]bool{codes.Unknown: true, codes.Internal: true, codes.Unimplemented: true, codes.Unavailable: true, codes.DataLoss: true}

	var methods int
	protoregistry.GlobalFiles.RangeFilesByPackage("skelet.signals.v1", func(fd protoreflect.FileDescriptor) bool {
		for i := 0; i < fd.Services().Len(); i++ {
			svc := fd.Services().Get(i)
			for j := 0; j < svc.Methods().Len(); j++ {
				m := svc.Methods().Get(j)
				if m.IsStreamingClient() || m.IsStreamingServer() {
					continue
				}
				methods++
				full := "/" + string(svc.FullName()) + "/" + string(m.Name())
				t.Run(strings.TrimPrefix(full, "/skelet.signals.v1."), func(t *testing.T) {
					code := invoke(cc, admin, full, m)
					if code == codes.PermissionDenied {
						code = invoke(cc, system, full, m)
					}
					if broken[code] {
						t.Errorf("%s answered %s to an empty request - the handler failed inside rather than refusing the input", full, code)
					}
				})
			}
		}
		return true
	})
	if methods == 0 {
		t.Fatal("no contract methods found - the generated package is not linked in")
	}
}

func invoke(cc *grpc.ClientConn, ctx context.Context, full string, m protoreflect.MethodDescriptor) codes.Code {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	err := cc.Invoke(ctx, full, dynamicpb.NewMessage(m.Input()), dynamicpb.NewMessage(m.Output()))
	return status.Code(err)
}

func tokenCtx(t *testing.T, secret string, claims auth.ServiceClaims) context.Context {
	t.Helper()
	tok, err := auth.GenerateServiceToken(claims, signalsapi.Audience, secret, time.Minute)
	if err != nil {
		t.Fatalf("mint token: %v", err)
	}
	return metadata.AppendToOutgoingContext(context.Background(), "authorization", "Bearer "+tok)
}
