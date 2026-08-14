package auth

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"strconv"
	"strings"
	"time"
)

type Permission string

const (
	Read    Permission = "read"
	Operate Permission = "operate"
	Probe   Permission = "probe"
	Restore Permission = "restore"
	Write   Permission = "write"
)

type Authorizer struct {
	AdminKey string
	Username string
	Password string
}

func (a Authorizer) signature(v string) string {
	h := hmac.New(sha256.New, a.sessionSecret())
	h.Write([]byte(v))
	return hex.EncodeToString(h.Sum(nil))
}
func (a Authorizer) sessionSecret() []byte {
	if a.AdminKey != "" {
		return []byte(a.AdminKey)
	}
	sum := sha256.Sum256([]byte(a.Username + "\x00" + a.Password))
	return sum[:]
}
func (a Authorizer) Login(w http.ResponseWriter, r *http.Request) bool {
	if r.Method != "POST" {
		return false
	}
	if a.Username == "" || a.Password == "" || r.FormValue("username") != a.Username || r.FormValue("password") != a.Password {
		return false
	}
	exp := strconv.FormatInt(time.Now().Add(2*time.Hour).Unix(), 10)
	http.SetCookie(w, &http.Cookie{Name: "xing_shu_admin", Value: exp + "." + a.signature(exp), Path: "/", HttpOnly: true, SameSite: http.SameSiteStrictMode, MaxAge: 7200})
	return true
}
func (a Authorizer) Check(r *http.Request, p Permission) bool {
	if a.AdminKey == "" && (a.Username == "" || a.Password == "") {
		return false
	}
	if a.AdminKey != "" {
		if h := r.Header.Get("Authorization"); h == "Bearer "+a.AdminKey {
			return true
		}
	}
	cookies := r.Cookies()
	for _, c := range cookies {
		if c.Name != "xing_shu_admin" {
			continue
		}
		x := strings.SplitN(c.Value, ".", 2)
		if len(x) != 2 {
			continue
		}
		exp, err := strconv.ParseInt(x[0], 10, 64)
		if err == nil && exp > time.Now().Unix() && hmac.Equal([]byte(x[1]), []byte(a.signature(x[0]))) {
			return true
		}
	}
	return false
}
