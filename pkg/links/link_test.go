package links_test

import (
	"testing"

	"github.com/can3p/pcom/pkg/links"
	"github.com/stretchr/testify/require"
)

func TestLink_SimpleNames(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		expected string
	}{
		{"controls", "/controls"},
		{"settings", "/controls/settings"},
		{"write", "/write"},
		{"feed", "/feed"},
		{"explore", "/explore"},
		{"privacy_policy", "/articles/privacy_policy"},
		{"terms_of_service", "/articles/terms_of_service"},
		{"login", "/login"},
		{"signup", "/signup"},
		{"form_signup_waiting_list", "/form/signup_waiting_list"},
		{"form_signup", "/form/signup"},
		{"form_login", "/form/login"},
		{"form_edit_post", "/controls/form/edit_post"},
		{"form_new_comment", "/controls/form/new_comment"},
		{"form_save_settings", "/controls/form/save_settings"},
		{"form_user_styles", "/controls/form/save_user_styles"},
		{"form_add_user_feed", "/controls/form/add_user_feed"},
		{"form_send_invite", "/controls/form/send_invite"},
		{"form_change_password", "/controls/form/change_password"},
		{"form_whitelist_connection", "/controls/form/whitelist_connection"},
		{"form_prompt_post", "/controls/form/prompt_post"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := links.Link(tt.name)
			require.Equal(t, tt.expected, got)
		})
	}
}

func TestLink_WithArguments(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		args     []string
		expected string
	}{
		{"post", []string{"123"}, "/posts/123"},
		{"shared_post", []string{"456"}, "/shared/456"},
		{"edit_post", []string{"789"}, "/posts/789/edit"},
		{"user", []string{"user-id-1"}, "/users/user-id-1"},
		{"user_styles", []string{"user-id-2"}, "/users/user-id-2/user_styles"},
		{"article", []string{"about"}, "/articles/about"},
		{"invite", []string{"invite-token"}, "/invite/invite-token"},
		{"use_case", []string{"use-case-slug"}, "/use-case/use-case-slug"},
		{"form_accept_invite", []string{"invite-token"}, "/form/accept_invite/invite-token"},
		{"confirm_waiting_list", []string{"token-123"}, "/confirm_waiting_list/token-123"},
		{"confirm_signup", []string{"token-456"}, "/confirm_signup/token-456"},
		{"action", []string{"publish"}, "/controls/action/publish"},
		{"uploaded_media", []string{"image.jpg"}, "/user-media/image.jpg"},
		{"public_blog_feed", []string{"user-1"}, "/rss/public/user-1"},
		{"private_user_feed", []string{"user-2"}, "/rss/private/user-2"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := links.Link(tt.name, tt.args...)
			require.Equal(t, tt.expected, got)
		})
	}
}

func TestLink_Comment_Fragment(t *testing.T) {
	t.Parallel()

	got := links.Link("comment", "post-123", "comment-456")
	// Should have the post URL with comment fragment
	require.Equal(t, "/posts/post-123#commentpost-123comment-456", got)
}

func TestLink_QueryString(t *testing.T) {
	t.Parallel()

	// Test that query strings are properly appended
	got := links.Link("post", "123", "key1", "value1", "key2", "value2")
	require.Equal(t, "/posts/123?key1=value1&key2=value2", got)
}

func TestLink_QueryStringAndFragment(t *testing.T) {
	t.Parallel()

	// Comment with query string
	got := links.Link("comment", "post-id", "comment-id", "ref", "123")
	require.Equal(t, "/posts/post-id?ref=123#commentpost-idcomment-id", got)
}

func TestDefaultAuthorizedHome(t *testing.T) {
	t.Parallel()

	got := links.DefaultAuthorizedHome()
	require.Equal(t, "/feed", got)
}

func TestLink_DefaultAuthorizedHome_Name(t *testing.T) {
	t.Parallel()

	got := links.Link("default_authorized_home")
	require.Equal(t, "/feed", got)
}

func TestAbsLink_Simple_WithSiteRoot(t *testing.T) {
	t.Setenv("SITE_ROOT", "https://example.com")

	got := links.AbsLink("feed")
	require.Equal(t, "https://example.com/feed", got)
}

func TestAbsLink_Simple_WithoutSiteRoot(t *testing.T) {
	t.Setenv("SITE_ROOT", "")

	got := links.AbsLink("feed")
	require.Equal(t, "/feed", got)
}

func TestAbsLink_UploadedMedia_WithCDN_InCluster(t *testing.T) {
	t.Setenv("USER_MEDIA_CDN", "https://cdn.example.com")
	t.Setenv("FLY_APP_NAME", "myapp")
	t.Setenv("SITE_ROOT", "https://example.com")

	got := links.AbsLink("uploaded_media", "image.jpg")
	require.Equal(t, "https://cdn.example.com/image.jpg", got)
}

func TestAbsLink_UploadedMedia_WithCDN_NotInCluster(t *testing.T) {
	t.Setenv("USER_MEDIA_CDN", "https://cdn.example.com")
	t.Setenv("SITE_ROOT", "https://example.com")

	got := links.AbsLink("uploaded_media", "image.jpg")
	require.Equal(t, "https://example.com/user-media/image.jpg", got)
}

func TestAbsLink_UploadedMedia_WithoutCDN(t *testing.T) {
	t.Setenv("SITE_ROOT", "https://example.com")

	got := links.AbsLink("uploaded_media", "image.jpg")
	require.Equal(t, "https://example.com/user-media/image.jpg", got)
}

func TestAbsLink_UploadedMedia_SingleFile(t *testing.T) {
	t.Setenv("SITE_ROOT", "https://example.com")

	got := links.AbsLink("uploaded_media", "file-uuid-123.png")
	require.Equal(t, "https://example.com/user-media/file-uuid-123.png", got)
}

func TestAbsLink_UploadedMedia_UUIDFile(t *testing.T) {
	t.Setenv("SITE_ROOT", "https://example.com")

	got := links.AbsLink("uploaded_media", "3fa85f64-5717-4562-b3fc-2c963f66afa6.jpg")
	require.Equal(t, "https://example.com/user-media/3fa85f64-5717-4562-b3fc-2c963f66afa6.jpg", got)
}

func TestAbsLink_Post_WithArguments(t *testing.T) {
	t.Setenv("SITE_ROOT", "https://example.com")

	got := links.AbsLink("post", "123")
	require.Equal(t, "https://example.com/posts/123", got)
}

func TestAbsLink_User_WithArguments(t *testing.T) {
	t.Setenv("SITE_ROOT", "https://example.com")

	got := links.AbsLink("user", "alice")
	require.Equal(t, "https://example.com/users/alice", got)
}

func TestAbsLink_WithQueryString(t *testing.T) {
	t.Setenv("SITE_ROOT", "https://example.com")

	got := links.AbsLink("feed", "sort", "recent")
	require.Equal(t, "https://example.com/feed?sort=recent", got)
}
