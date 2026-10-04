package service_test

import (
	"context"
	"errors"
	"testing"

	"github.com/AXmishell/axmipic/internal/auth"
	"github.com/AXmishell/axmipic/internal/service"
	"github.com/AXmishell/axmipic/internal/store"
)

func TestAnnouncementLifecycle(t *testing.T) {
	repo := newRepo(t)
	svc := service.NewSiteService(repo)
	ctx := context.Background()

	draft, err := svc.CreateAnnouncement(ctx, service.AnnouncementInput{Title: "维护通知", Content: "今晚维护", Level: "warning"})
	if err != nil {
		t.Fatalf("CreateAnnouncement: %v", err)
	}
	if draft.Level != "warning" || !draft.Published {
		t.Fatalf("announcement = %+v", draft)
	}

	// 公开列表仅返回已发布公告。
	public, err := svc.ListAnnouncements(ctx, true)
	if err != nil || len(public) != 1 {
		t.Fatalf("public announcements = %+v, err = %v", public, err)
	}

	updated, err := svc.UpdateAnnouncement(ctx, draft.ID, service.AnnouncementInput{
		Title: "维护通知", Content: "改期", Level: "info", Published: false,
	})
	if err != nil {
		t.Fatalf("UpdateAnnouncement: %v", err)
	}
	if updated.Published {
		t.Fatalf("expected unpublished: %+v", updated)
	}
	public, _ = svc.ListAnnouncements(ctx, true)
	if len(public) != 0 {
		t.Fatalf("public announcements after unpublish = %+v", public)
	}

	if err := svc.DeleteAnnouncement(ctx, draft.ID); err != nil {
		t.Fatalf("DeleteAnnouncement: %v", err)
	}
	if err := svc.DeleteAnnouncement(ctx, draft.ID); !errors.Is(err, service.ErrAnnouncementNotFound) {
		t.Fatalf("delete again err = %v, want ErrAnnouncementNotFound", err)
	}

	if _, err := svc.CreateAnnouncement(ctx, service.AnnouncementInput{Title: ""}); !errors.Is(err, service.ErrInvalidInput) {
		t.Fatalf("empty title err = %v, want ErrInvalidInput", err)
	}
	if _, err := svc.CreateAnnouncement(ctx, service.AnnouncementInput{Title: "x", Level: "nope"}); !errors.Is(err, service.ErrInvalidInput) {
		t.Fatalf("bad level err = %v, want ErrInvalidInput", err)
	}
}

func TestReportLifecycle(t *testing.T) {
	repo := newRepo(t)
	newCustomer(t, repo, "u1")
	upload := service.NewUploadService(repo, managerWithFallback(t, newFakeStorage()), pngPolicy())
	svc := service.NewSiteService(repo)
	ctx := context.Background()
	u1 := &auth.Principal{UserID: "u1", Username: "u1", Role: auth.RoleUser}

	img, err := upload.Upload(ctx, u1, service.UploadInput{Data: testPNGSize(t, 8), MimeType: "image/png"})
	if err != nil {
		t.Fatalf("Upload: %v", err)
	}

	report, err := svc.CreateReport(ctx, u1, service.ReportInput{ImageID: img.ID, Reason: "侵权", Detail: "未经授权"})
	if err != nil {
		t.Fatalf("CreateReport: %v", err)
	}
	if report.Status != store.ReportPending || report.ReporterName != "u1" {
		t.Fatalf("report = %+v", report)
	}

	list, err := svc.ListReports(ctx, store.ReportPending)
	if err != nil || len(list) != 1 {
		t.Fatalf("pending reports = %+v, err = %v", list, err)
	}

	updated, err := svc.UpdateReportStatus(ctx, report.ID, store.ReportResolved, "已删除")
	if err != nil {
		t.Fatalf("UpdateReportStatus: %v", err)
	}
	if updated.Status != store.ReportResolved || updated.HandlerNote != "已删除" {
		t.Fatalf("updated report = %+v", updated)
	}
	if _, err := svc.UpdateReportStatus(ctx, report.ID, "bogus", ""); !errors.Is(err, service.ErrInvalidInput) {
		t.Fatalf("bad status err = %v, want ErrInvalidInput", err)
	}
	if _, err := svc.CreateReport(ctx, u1, service.ReportInput{ImageID: "missing", Reason: "x"}); !errors.Is(err, service.ErrNotFound) {
		t.Fatalf("missing image err = %v, want ErrNotFound", err)
	}
	if _, err := svc.CreateReport(ctx, u1, service.ReportInput{Reason: ""}); !errors.Is(err, service.ErrInvalidInput) {
		t.Fatalf("empty reason err = %v, want ErrInvalidInput", err)
	}
}

func TestPageLifecycle(t *testing.T) {
	repo := newRepo(t)
	svc := service.NewSiteService(repo)
	ctx := context.Background()

	page, err := svc.CreatePage(ctx, service.PageInput{Slug: "About-Us", Title: "关于我们", Content: "内容", Published: true})
	if err != nil {
		t.Fatalf("CreatePage: %v", err)
	}
	if page.Slug != "about-us" {
		t.Fatalf("slug not normalized: %q", page.Slug)
	}
	got, err := svc.GetPageBySlug(ctx, "about-us")
	if err != nil || got.ID != page.ID {
		t.Fatalf("GetPageBySlug = %+v, err = %v", got, err)
	}

	// 未发布页面不对外可见，但管理端可见。
	if _, err := svc.UpdatePage(ctx, page.ID, service.PageInput{Slug: "about-us", Title: "关于", Published: false}); err != nil {
		t.Fatalf("UpdatePage: %v", err)
	}
	if _, err := svc.GetPageBySlug(ctx, "about-us"); !errors.Is(err, service.ErrPageNotFound) {
		t.Fatalf("unpublished page err = %v, want ErrPageNotFound", err)
	}
	all, err := svc.ListPages(ctx, false)
	if err != nil || len(all) != 1 {
		t.Fatalf("admin pages = %+v, err = %v", all, err)
	}

	if _, err := svc.CreatePage(ctx, service.PageInput{Slug: "Bad Slug!", Title: "x"}); !errors.Is(err, service.ErrInvalidInput) {
		t.Fatalf("bad slug err = %v, want ErrInvalidInput", err)
	}
	if _, err := svc.CreatePage(ctx, service.PageInput{Slug: "about-us", Title: "重复"}); !errors.Is(err, service.ErrInvalidInput) {
		t.Fatalf("duplicate slug err = %v, want ErrInvalidInput", err)
	}
}
