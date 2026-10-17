package agent

import "testing"

// 404 页面写入接口只接受绝对路径 + .html/.htm，避免被当成任意文件写入。
func TestValidatePagePath(t *testing.T) {
	ok := []string{"/etc/frp/havline-404.html", "/var/www/404.htm"}
	for _, path := range ok {
		if _, err := validatePagePath(path); err != nil {
			t.Errorf("%s 应当通过：%v", path, err)
		}
	}
	bad := []string{"", "etc/frp/404.html", "/etc/frp/404.txt", "/etc/../etc/passwd.html"}
	for _, path := range bad {
		if _, err := validatePagePath(path); err == nil {
			t.Errorf("%s 应当被拒绝", path)
		}
	}
}
