// Copyright 2011 Gary Burd
//
// Licensed under the Apache License, Version 2.0 (the "License"): you may
// not use this file except in compliance with the License. You may obtain
// a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS, WITHOUT
// WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied. See the
// License for the specific language governing permissions and limitations
// under the License.

package doc

import (
	"encoding/xml"
	"errors"
	"net/http"
	"path"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"
)

// service represents a source code control service.
type service struct {
	pattern *regexp.Regexp
	getDoc  func(*http.Client, []string, string) (*Package, error)
	prefix  string
}

// services is the list of source code control services handled by gopkgdoc.
var services = []*service{
	&service{githubPattern, getGithubDoc, "github.com/"},
	&service{googlePattern, getGoogleDoc, "code.google.com/"},
	&service{bitbucketPattern, getBitbucketDoc, "bitbucket.org/"},
	&service{launchpadPattern, getLaunchpadDoc, "launchpad.net/"},
	&service{gitoriousPattern, getGitoriousDoc, "git.gitorious.org/"},
}

func attrValue(attrs []xml.Attr, name string) string {
	for _, a := range attrs {
		if strings.EqualFold(a.Name.Local, name) {
			return a.Value
		}
	}
	return ""
}

func getMeta(client *http.Client, importPath string) (projectRoot, projectName, projectURL, repoRoot string, err error) {
	var resp *http.Response

	uri := importPath
	if !strings.Contains(uri, "/") {
		// Add slash for root of domain.
		uri = uri + "/"
	}
	uri = uri + "?go-get=1"

	proto := "https://"
	resp, err = client.Get(proto + uri)
	if err != nil || resp.StatusCode != 200 {
		if err == nil {
			resp.Body.Close()
		}
		proto = "http://"
		resp, err = client.Get(proto + uri)
		if err != nil {
			err = GetError{strings.SplitN(importPath, "/", 2)[0], err}
			return
		}
	}
	defer resp.Body.Close()

	d := xml.NewDecoder(resp.Body)
	d.Strict = false

	err = ErrPackageNotFound
	for {
		t, tokenErr := d.Token()
		if tokenErr != nil {
			break
		}
		switch t := t.(type) {
		case xml.EndElement:
			if strings.EqualFold(t.Name.Local, "head") {
				return
			}
		case xml.StartElement:
			if strings.EqualFold(t.Name.Local, "body") {
				return
			}
			if !strings.EqualFold(t.Name.Local, "meta") ||
				attrValue(t.Attr, "name") != "go-import" {
				continue
			}
			f := strings.Fields(attrValue(t.Attr, "content"))
			if len(f) != 3 ||
				!strings.HasPrefix(importPath, f[0]) ||
				!(len(importPath) == len(f[0]) || importPath[len(f[0])] == '/') {
				continue
			}
			if err == nil {
				// More than one matching meta tag. Handle as not found.
				err = ErrPackageNotFound
				return
			}
			err = nil
			projectRoot = f[0]
			repoRoot = f[2]
			_, projectName = path.Split(projectRoot)
			projectURL = proto + projectRoot
		}
	}
	return
}

// getDynamic gets a document from a service that is not statically known.
func getDynamic(client *http.Client, importPath string, etag string) (*Package, error) {
	projectRoot, projectName, projectURL, repoRoot, err := getMeta(client, importPath)
	if err != nil {
		return nil, err
	}

	if projectRoot != importPath {
		var projectRoot2 string
		projectRoot2, projectName, projectURL, _, err = getMeta(client, projectRoot)
		if err != nil {
			return nil, err
		}
		if projectRoot2 != projectRoot {
			return nil, ErrPackageNotFound
		}
	}

	i := strings.Index(repoRoot, "://")
	if i < 0 {
		return nil, ErrPackageNotFound
	}
	importPath2 := repoRoot[i+len("://"):] + importPath[len(projectRoot):]

	pdoc, err := getStatic(client, importPath2, etag)

	if err == nil {
		pdoc.ImportPath = importPath
		pdoc.ProjectRoot = projectRoot
		pdoc.ProjectName = projectName
		pdoc.ProjectURL = projectURL
		return pdoc, err
	}

	if err == errNoMatch {
		return getProxyDoc(client, importPath, projectRoot, projectName, projectURL, etag)
	}

	return nil, err
}

var errNoMatch = errors.New("no match")

// getStatic gets a document from a statically known service. getStatic returns
// errNoMatch if the import path is not recognized.
func getStatic(client *http.Client, importPath string, etag string) (*Package, error) {
	for _, s := range services {
		if !strings.HasPrefix(importPath, s.prefix) {
			continue
		}
		m := s.pattern.FindStringSubmatch(importPath)
		if m == nil && s.prefix != "" {
			// Import path is bad if prefix matches and regexp does not.
			return nil, ErrPackageNotFound
		}
		return s.getDoc(client, m, etag)
	}
	return nil, errNoMatch
}

func Get(client *http.Client, importPath string, etag string) (pdoc *Package, err error) {

	const versionPrefix = PackageVersion + "-"

	if strings.HasPrefix(etag, versionPrefix) {
		etag = etag[len(versionPrefix):]
	} else {
		etag = ""
	}

	switch {
	case StandardPackages[importPath]:
		pdoc, err = getStandardDoc(client, importPath, etag)
	case !ValidRemotePath(importPath):
		return nil, ErrPackageNotFound
	default:
		pdoc, err = getStatic(client, importPath, etag)
		if err == errNoMatch {
			pdoc, err = getDynamic(client, importPath, etag)
		}
	}

	if err == nil {
		pdoc.Etag = versionPrefix + pdoc.Etag
	}

	return pdoc, err
}

var (
	ErrPackageNotFound    = errors.New("package not found")
	ErrPackageNotModified = errors.New("package not modified")
)

var validHost = regexp.MustCompile(`^[-A-Za-z0-9]+(?:\.[-A-Za-z0-9]+)+`)

var validTLD = map[string]bool{
	// curl http://data.iana.org/TLD/tlds-alpha-by-domain.txt | sed  -e '/#/ d' -e 's/.*/"&": true,/' | tr [:upper:] [:lower:]
	".ac":                     true,
	".ad":                     true,
	".ae":                     true,
	".aero":                   true,
	".af":                     true,
	".ag":                     true,
	".ai":                     true,
	".al":                     true,
	".am":                     true,
	".an":                     true,
	".ao":                     true,
	".aq":                     true,
	".ar":                     true,
	".arpa":                   true,
	".as":                     true,
	".asia":                   true,
	".at":                     true,
	".au":                     true,
	".aw":                     true,
	".ax":                     true,
	".az":                     true,
	".ba":                     true,
	".bb":                     true,
	".bd":                     true,
	".be":                     true,
	".bf":                     true,
	".bg":                     true,
	".bh":                     true,
	".bi":                     true,
	".biz":                    true,
	".bj":                     true,
	".bm":                     true,
	".bn":                     true,
	".bo":                     true,
	".br":                     true,
	".bs":                     true,
	".bt":                     true,
	".bv":                     true,
	".bw":                     true,
	".by":                     true,
	".bz":                     true,
	".ca":                     true,
	".cat":                    true,
	".cc":                     true,
	".cd":                     true,
	".cf":                     true,
	".cg":                     true,
	".ch":                     true,
	".ci":                     true,
	".ck":                     true,
	".cl":                     true,
	".cm":                     true,
	".cn":                     true,
	".co":                     true,
	".com":                    true,
	".coop":                   true,
	".cr":                     true,
	".cu":                     true,
	".cv":                     true,
	".cw":                     true,
	".cx":                     true,
	".cy":                     true,
	".cz":                     true,
	".de":                     true,
	".dj":                     true,
	".dk":                     true,
	".dm":                     true,
	".do":                     true,
	".dz":                     true,
	".ec":                     true,
	".edu":                    true,
	".ee":                     true,
	".eg":                     true,
	".er":                     true,
	".es":                     true,
	".et":                     true,
	".eu":                     true,
	".fi":                     true,
	".fj":                     true,
	".fk":                     true,
	".fm":                     true,
	".fo":                     true,
	".fr":                     true,
	".ga":                     true,
	".gb":                     true,
	".gd":                     true,
	".ge":                     true,
	".gf":                     true,
	".gg":                     true,
	".gh":                     true,
	".gi":                     true,
	".gl":                     true,
	".gm":                     true,
	".gn":                     true,
	".gov":                    true,
	".gp":                     true,
	".gq":                     true,
	".gr":                     true,
	".gs":                     true,
	".gt":                     true,
	".gu":                     true,
	".gw":                     true,
	".gy":                     true,
	".hk":                     true,
	".hm":                     true,
	".hn":                     true,
	".hr":                     true,
	".ht":                     true,
	".hu":                     true,
	".id":                     true,
	".ie":                     true,
	".il":                     true,
	".im":                     true,
	".in":                     true,
	".info":                   true,
	".int":                    true,
	".io":                     true,
	".iq":                     true,
	".ir":                     true,
	".is":                     true,
	".it":                     true,
	".je":                     true,
	".jm":                     true,
	".jo":                     true,
	".jobs":                   true,
	".jp":                     true,
	".ke":                     true,
	".kg":                     true,
	".kh":                     true,
	".ki":                     true,
	".km":                     true,
	".kn":                     true,
	".kp":                     true,
	".kr":                     true,
	".kw":                     true,
	".ky":                     true,
	".kz":                     true,
	".la":                     true,
	".lb":                     true,
	".lc":                     true,
	".li":                     true,
	".lk":                     true,
	".lr":                     true,
	".ls":                     true,
	".lt":                     true,
	".lu":                     true,
	".lv":                     true,
	".ly":                     true,
	".ma":                     true,
	".mc":                     true,
	".md":                     true,
	".me":                     true,
	".mg":                     true,
	".mh":                     true,
	".mil":                    true,
	".mk":                     true,
	".ml":                     true,
	".mm":                     true,
	".mn":                     true,
	".mo":                     true,
	".mobi":                   true,
	".mp":                     true,
	".mq":                     true,
	".mr":                     true,
	".ms":                     true,
	".mt":                     true,
	".mu":                     true,
	".museum":                 true,
	".mv":                     true,
	".mw":                     true,
	".mx":                     true,
	".my":                     true,
	".mz":                     true,
	".na":                     true,
	".name":                   true,
	".nc":                     true,
	".ne":                     true,
	".net":                    true,
	".nf":                     true,
	".ng":                     true,
	".ni":                     true,
	".nl":                     true,
	".no":                     true,
	".np":                     true,
	".nr":                     true,
	".nu":                     true,
	".nz":                     true,
	".om":                     true,
	".org":                    true,
	".pa":                     true,
	".pe":                     true,
	".pf":                     true,
	".pg":                     true,
	".ph":                     true,
	".pk":                     true,
	".pl":                     true,
	".pm":                     true,
	".pn":                     true,
	".post":                   true,
	".pr":                     true,
	".pro":                    true,
	".ps":                     true,
	".pt":                     true,
	".pw":                     true,
	".py":                     true,
	".qa":                     true,
	".re":                     true,
	".ro":                     true,
	".rs":                     true,
	".ru":                     true,
	".rw":                     true,
	".sa":                     true,
	".sb":                     true,
	".sc":                     true,
	".sd":                     true,
	".se":                     true,
	".sg":                     true,
	".sh":                     true,
	".si":                     true,
	".sj":                     true,
	".sk":                     true,
	".sl":                     true,
	".sm":                     true,
	".sn":                     true,
	".so":                     true,
	".sr":                     true,
	".st":                     true,
	".su":                     true,
	".sv":                     true,
	".sx":                     true,
	".sy":                     true,
	".sz":                     true,
	".tc":                     true,
	".td":                     true,
	".tel":                    true,
	".tf":                     true,
	".tg":                     true,
	".th":                     true,
	".tj":                     true,
	".tk":                     true,
	".tl":                     true,
	".tm":                     true,
	".tn":                     true,
	".to":                     true,
	".tp":                     true,
	".tr":                     true,
	".travel":                 true,
	".tt":                     true,
	".tv":                     true,
	".tw":                     true,
	".tz":                     true,
	".ua":                     true,
	".ug":                     true,
	".uk":                     true,
	".us":                     true,
	".uy":                     true,
	".uz":                     true,
	".va":                     true,
	".vc":                     true,
	".ve":                     true,
	".vg":                     true,
	".vi":                     true,
	".vn":                     true,
	".vu":                     true,
	".wf":                     true,
	".ws":                     true,
	".xn--0zwm56d":            true,
	".xn--11b5bs3a9aj6g":      true,
	".xn--3e0b707e":           true,
	".xn--45brj9c":            true,
	".xn--80akhbyknj4f":       true,
	".xn--80ao21a":            true,
	".xn--90a3ac":             true,
	".xn--9t4b11yi5a":         true,
	".xn--clchc0ea0b2g2a9gcd": true,
	".xn--deba0ad":            true,
	".xn--fiqs8s":             true,
	".xn--fiqz9s":             true,
	".xn--fpcrj9c3d":          true,
	".xn--fzc2c9e2c":          true,
	".xn--g6w251d":            true,
	".xn--gecrj9c":            true,
	".xn--h2brj9c":            true,
	".xn--hgbk6aj7f53bba":     true,
	".xn--hlcj6aya9esc7a":     true,
	".xn--j6w193g":            true,
	".xn--jxalpdlp":           true,
	".xn--kgbechtv":           true,
	".xn--kprw13d":            true,
	".xn--kpry57d":            true,
	".xn--lgbbat1ad8j":        true,
	".xn--mgb9awbf":           true,
	".xn--mgbaam7a8h":         true,
	".xn--mgbayh7gpa":         true,
	".xn--mgbbh1a71e":         true,
	".xn--mgbc0a9azcg":        true,
	".xn--mgberp4a5d4ar":      true,
	".xn--mgbx4cd0ab":         true,
	".xn--o3cw4h":             true,
	".xn--ogbpf8fl":           true,
	".xn--p1ai":               true,
	".xn--pgbs0dh":            true,
	".xn--s9brj9c":            true,
	".xn--wgbh1c":             true,
	".xn--wgbl6a":             true,
	".xn--xkc2al3hye2a":       true,
	".xn--xkc2dl3a5ee0h":      true,
	".xn--yfro4i67o":          true,
	".xn--ygbi2ammx":          true,
	".xn--zckzah":             true,
	".xxx":                    true,
	".ye":                     true,
	".yt":                     true,
	".za":                     true,
	".zm":                     true,
	".zw":                     true,
}

// ValidRemotePath returns true if importPath is structurally valid for "go get".
func ValidRemotePath(importPath string) bool {

	// See isbadimport in $GOROOT/src/cmd/gc/subr.c for rune checks.
	for _, r := range importPath {
		if r == utf8.RuneError {
			return false
		}
		if r < 0x20 || r == 0x7f {
			return false
		}
		if r == '\\' {
			return false
		}
		if unicode.IsSpace(r) {
			return false
		}
		if strings.IndexRune("!\"#$%&'()*,:;<=>?[]^`{|}", r) >= 0 {
			return false
		}
	}

	parts := strings.Split(importPath, "/")

	if !validTLD[path.Ext(parts[0])] {
		return false
	}

	if !validHost.MatchString(parts[0]) {
		return false
	}

	for _, part := range parts[1:] {
		if len(part) == 0 ||
			part[0] == '.' ||
			part[0] == '_' ||
			part == "testdata" {
			return false
		}
	}

	if i := strings.Index(importPath, "/src/pkg/"); i > 0 && StandardPackages[importPath[i+len("/src/pkg/"):]] {
		return false
	}

	return true
}
