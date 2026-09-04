package service_test

import (
	"context"
	"testing"

	"github.com/ramisoul84/task-management/internal/domain"
	"github.com/ramisoul84/task-management/internal/service"
)

func TestUserService(t *testing.T) {
	ctx := context.Background()
	s := newStore()
	s.addUser("rami")
	s.users["rami"].PasswordHash = "hashed:old-password"
	svc := service.NewUserService(userRepo{s}, &plainHasher{})

	me, err := svc.Me(ctx, "rami")
	wantErr(t, err, nil)
	if me.ID != "rami" {
		t.Fatalf("me = %+v", me)
	}

	me, err = svc.UpdateProfile(ctx, "rami", &domain.UpdateProfileRequest{Name: "  Rami S. "})
	wantErr(t, err, nil)
	if me.Name != "Rami S." {
		t.Fatalf("name = %q", me.Name)
	}

	err = svc.ChangePassword(ctx, "rami", &domain.ChangePasswordRequest{CurrentPassword: "wrong", NewPassword: "new-password"})
	wantErr(t, err, domain.ErrInvalidCredentials)
	if s.users["rami"].PasswordHash != "hashed:old-password" {
		t.Fatal("password must not change when the current one is wrong")
	}

	err = svc.ChangePassword(ctx, "rami", &domain.ChangePasswordRequest{CurrentPassword: "old-password", NewPassword: "new-password"})
	wantErr(t, err, nil)
	if s.users["rami"].PasswordHash != "hashed:new-password" {
		t.Fatal("password not updated")
	}
}
