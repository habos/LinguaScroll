package config

import "testing"

func TestLoadDefaultsToFakesWithoutKey(t *testing.T) {
	t.Setenv("YOUTUBE_API_KEY", "")
	t.Setenv("FAKE_VENDORS", "")
	c, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if !c.FakeVendors || c.HTTPAddr != ":8080" || c.RateLimitPerMinute != 120 {
		t.Fatalf("unexpected defaults: %+v", c)
	}
}

func TestLoadRealVendorsNeedKey(t *testing.T) {
	t.Setenv("YOUTUBE_API_KEY", "")
	t.Setenv("FAKE_VENDORS", "0")
	if _, err := Load(); err == nil {
		t.Fatal("expected an error without YOUTUBE_API_KEY")
	}
	t.Setenv("YOUTUBE_API_KEY", "k")
	c, err := Load()
	if err != nil || c.FakeVendors {
		t.Fatalf("got %+v, %v", c, err)
	}
}

func TestLoadProdNeedsAdmin(t *testing.T) {
	t.Setenv("APP_ENV", "prod")
	t.Setenv("FAKE_VENDORS", "1")
	t.Setenv("ADMIN_USER", "")
	if _, err := Load(); err == nil {
		t.Fatal("expected an error without admin credentials in prod")
	}
}
