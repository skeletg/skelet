package admin

import (
	"context"
	"reflect"
	"runtime/debug"
	"strings"
	"testing"

	"github.com/google/uuid"
)

// A resolver with no services wired panics on its first load, so a panic under an anonymous caller means it loaded before refusing
func TestNoAdminResolverLoadsBeforeRefusingAnonymousCallers(t *testing.T) {
	ctxType := reflect.TypeFor[context.Context]()
	checked := 0
	for _, recv := range []any{&mutationResolver{&Resolver{}}, &queryResolver{&Resolver{}}} {
		v := reflect.ValueOf(recv)
		for m := range v.Type().Methods() {
			if _, ok := publicResolvers[m.Name]; ok {
				continue
			}
			if m.Type.NumIn() < 2 || m.Type.In(1) != ctxType {
				continue
			}
			args := []reflect.Value{v, reflect.ValueOf(context.Background())}
			for j := 2; j < m.Type.NumIn(); j++ {
				args = append(args, placeholderArg(m.Type.In(j)))
			}
			checked++
			if site := callForPanic(m.Func, args); site != "" {
				t.Errorf("%s reads through %s before refusing an anonymous caller - put a permission gate ahead of the load", m.Name, site)
			}
		}
	}
	if checked < 200 {
		t.Fatalf("called only %d resolvers", checked)
	}
}

func callForPanic(fn reflect.Value, args []reflect.Value) (site string) {
	defer func() {
		if recover() != nil {
			site = panicSite(string(debug.Stack()))
		}
	}()
	fn.Call(args)
	return ""
}

// Ids are valid uuids so parsing passes and the resolver reaches its first load
func placeholderArg(t reflect.Type) reflect.Value {
	switch {
	case t.Kind() == reflect.String:
		return reflect.ValueOf(uuid.NewString()).Convert(t)
	case t.Kind() == reflect.Pointer && t.Elem().Kind() == reflect.String:
		s := reflect.New(t.Elem())
		s.Elem().Set(reflect.ValueOf(uuid.NewString()).Convert(t.Elem()))
		return s
	case t.Kind() == reflect.Pointer && t.Elem().Kind() == reflect.Struct:
		return reflect.New(t.Elem())
	}
	return reflect.Zero(t)
}

func panicSite(stack string) string {
	const pkg = "core/graph/admin."
	past := false
	for l := range strings.SplitSeq(stack, "\n") {
		if strings.HasPrefix(l, "panic(") {
			past = true
			continue
		}
		_, fn, found := strings.Cut(l, pkg)
		if !past || !found {
			continue
		}
		if j := strings.LastIndex(fn, "("); j > 0 {
			fn = fn[:j]
		}
		if !strings.Contains(fn, ".func") {
			return fn
		}
	}
	return "an unwired service"
}

func TestScopeHelpersRefuseAnonymousCallersBeforeLoading(t *testing.T) {
	r := &Resolver{}
	for name, call := range map[string]func() error{
		"requireGiftCardByID": func() error { return r.requireGiftCardByID(context.Background(), "giftcards.manage", uuid.New()) },
		"requireWebhookScope": func() error {
			_, err := r.requireWebhookScope(context.Background(), uuid.New(), "webhooks.manage")
			return err
		},
		"requireWebhookQueueItemScope": func() error {
			_, err := r.requireWebhookQueueItemScope(context.Background(), uuid.New(), "webhooks.manage")
			return err
		},
		"requireWebhookDeliveryScope": func() error {
			_, err := r.requireWebhookDeliveryScope(context.Background(), uuid.New(), "webhooks.view")
			return err
		},
	} {
		if site := callForPanic(reflect.ValueOf(call), nil); site != "" {
			t.Errorf("%s loaded through %s before refusing an anonymous caller", name, site)
		}
	}
}
