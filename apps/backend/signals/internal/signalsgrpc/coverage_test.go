package signalsgrpc

import (
	"context"
	"errors"
	"net"
	"strings"
	"testing"

	"github.com/google/uuid"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/reflect/protoregistry"
	"google.golang.org/protobuf/types/dynamicpb"

	signalsv1 "github.com/skeletg/skelet/apps/backend/core/signalsapi/v1"
	"github.com/skeletg/skelet/apps/backend/signals/internal/analytics"
	"github.com/skeletg/skelet/apps/backend/signals/internal/campaign"
	"github.com/skeletg/skelet/apps/backend/signals/internal/emailtemplate"
	"github.com/skeletg/skelet/apps/backend/signals/internal/flow"
)

// The interceptor never blocks, so every handler refuses on its own - this walks every method the server registers, so an RPC added without its gate fails here instead of answering anyone who reaches the port
// Services are wired as empty values: a handler that touched one before refusing panics, which recovery turns into INTERNAL, and that fails too
func TestEveryRPCRefusesAnAnonymousCaller(t *testing.T) {
	srv := NewServer(Deps{
		ServiceKeys: testKeys, Channels: analytics.NewChannelRoster(nil),
		Flows: &flow.Service{}, Campaigns: &campaign.Service{}, Builder: &emailtemplate.Service{},
	})
	lis := bufconn.Listen(1 << 20)
	go func() { _ = srv.Serve(lis) }()
	t.Cleanup(srv.Stop)
	conn, err := grpc.NewClient("passthrough:///bufnet",
		grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) { return lis.DialContext(ctx) }),
		grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	refused := map[codes.Code]bool{codes.Unauthenticated: true, codes.PermissionDenied: true, codes.InvalidArgument: true}
	checked := 0
	for name, info := range srv.GetServiceInfo() {
		if !strings.HasPrefix(name, "skelet.signals.") {
			continue // health and reflection are deliberately open
		}
		d, err := protoregistry.GlobalFiles.FindDescriptorByName(protoreflect.FullName(name))
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		methods := d.(protoreflect.ServiceDescriptor).Methods()
		for _, m := range info.Methods {
			md := methods.ByName(protoreflect.Name(m.Name))
			full := "/" + name + "/" + m.Name
			// Empty, and again with every id-shaped field filled - a handler without its gate gets past parsing on the second and reaches its empty service
			for _, fill := range []bool{false, true} {
				code := status.Code(callAnonymously(t.Context(), conn, full, md, fill))
				if !refused[code] {
					t.Errorf("%s (filled %v) answered an anonymous caller with %s - it needs its permission check before anything else", full, fill, code)
				}
			}
			checked++
		}
	}
	if checked < 100 {
		t.Fatalf("only %d methods checked - did a service stop registering?", checked)
	}
}

func callAnonymously(ctx context.Context, conn *grpc.ClientConn, method string, md protoreflect.MethodDescriptor, fill bool) error {
	in, out := dynamicpb.NewMessage(md.Input()), dynamicpb.NewMessage(md.Output())
	if fill {
		fillIDs(in, 0)
	}
	if !md.IsStreamingClient() && !md.IsStreamingServer() {
		return conn.Invoke(ctx, method, in, out)
	}
	stream, err := conn.NewStream(ctx, &grpc.StreamDesc{ServerStreams: md.IsStreamingServer(), ClientStreams: md.IsStreamingClient()}, method)
	if err != nil {
		return err
	}
	if err := stream.SendMsg(in); err != nil {
		return err
	}
	if err := stream.CloseSend(); err != nil {
		return err
	}
	return stream.RecvMsg(out)
}

type anyCustomerRoster struct{}

func (anyCustomerRoster) ChannelForCustomer(context.Context, uuid.UUID) (uuid.UUID, error) {
	return uuid.Nil, errors.New("no such customer")
}

// A lookup ahead of the permission check would answer NOT_FOUND for an unknown customer and something else for a known one, so an anonymous caller could test which customers exist
func TestCustomerEngagementChecksTheCallerBeforeLookingUp(t *testing.T) {
	s := &pulseServer{roster: anyCustomerRoster{}}
	_, err := s.GetCustomerEngagement(context.Background(), &signalsv1.GetCustomerEngagementRequest{CustomerId: uuid.NewString()})
	if code := status.Code(err); code != codes.Unauthenticated && code != codes.PermissionDenied {
		t.Fatalf("anonymous lookup answered %s, want a refusal before the roster is asked", code)
	}
}

// fillIDs sets every singular string to a fresh uuid, two levels deep, so a handler's id parsing succeeds
func fillIDs(m protoreflect.Message, depth int) {
	fields := m.Descriptor().Fields()
	for i := range fields.Len() {
		f := fields.Get(i)
		if f.IsList() || f.IsMap() {
			continue
		}
		switch f.Kind() {
		case protoreflect.StringKind:
			m.Set(f, protoreflect.ValueOfString(uuid.NewString()))
		case protoreflect.MessageKind:
			if depth < 2 && f.Message().FullName().Parent() == "skelet.signals.v1" {
				fillIDs(m.Mutable(f).Message(), depth+1)
			}
		}
	}
}
