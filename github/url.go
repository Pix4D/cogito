package github

import (
	"errors"
	"fmt"
	"net/url"
	"strings"
)

type GitURL struct {
	URL      *url.URL
	Owner    string
	Repo     string
	FullName string
}

// safeUrlParse wraps [url.Parse] and returns only the error and not the URL to avoid leaking
// passwords of the form http://user:password@example.com
//
// From https://github.com/golang/go/issues/53993
func safeUrlParse(rawURL string) (*url.URL, error) {
	parsedUrl, err := url.Parse(rawURL)
	if err != nil {
		var uerr *url.Error
		if errors.As(err, &uerr) {
			// url.Parse returns a wrapped error that contains also the URL.
			// Instead, we return only the error.
			return nil, uerr.Err
		}
		return nil, errors.New("invalid URL")
	}
	return parsedUrl, nil
}

// ParseGitPseudoURL attempts to parse rawURL as a git remote URL compatible with the
// Github naming conventions.
//
// It supports the following types of git pseudo URLs:
//   - ssh:   			git@github.com:Pix4D/cogito.git; will be rewritten to the valid URL
//     ssh://git@github.com/Pix4D/cogito.git
//   - https: 			https://github.com/Pix4D/cogito.git
//   - https with u:p: 	https//username:password@github.com/Pix4D/cogito.git
//   - http: 			http://github.com/Pix4D/cogito.git
//   - http with u:p: 	http://username:password@github.com/Pix4D/cogito.git
func ParseGitPseudoURL(rawURL string) (GitURL, error) {
	workURL := rawURL
	// If ssh pseudo URL, we need to massage the rawURL ourselves :-(
	if strings.HasPrefix(workURL, "git@") {
		if strings.Count(workURL, ":") != 1 {
			return GitURL{}, fmt.Errorf("invalid git SSH URL %s: want exactly one ':'", rawURL)
		}
		// Make the URL a real URL, ready to be parsed. For example:
		// git@github.com:Pix4D/cogito.git -> ssh://git@github.com/Pix4D/cogito.git
		workURL = "ssh://" + strings.Replace(workURL, ":", "/", 1)
	}

	anyUrl, err := safeUrlParse(workURL)
	if err != nil {
		return GitURL{}, err
	}

	scheme := anyUrl.Scheme
	if scheme == "" {
		return GitURL{}, fmt.Errorf("invalid git URL %s: missing scheme", rawURL)
	}
	if scheme != "ssh" && scheme != "http" && scheme != "https" {
		return GitURL{}, fmt.Errorf("invalid git URL %s: invalid scheme: %s", rawURL, scheme)
	}

	// Further parse the path component of the URL to see if it complies with the GitHub
	// naming conventions.
	// Example of compliant path: github.com/Pix4D/cogito.git
	tokens := strings.Split(anyUrl.Path, "/")
	if have, want := len(tokens), 3; have != want {
		return GitURL{},
			fmt.Errorf("invalid git URL: path: want: %d components; have: %d %s",
				want, have, tokens)
	}

	owner := tokens[1]
	repo := strings.TrimSuffix(tokens[2], ".git")
	// All OK. Fill our gitURL struct
	gu := GitURL{
		URL:      anyUrl,
		Owner:    owner,
		Repo:     repo,
		FullName: owner + "/" + repo,
	}
	return gu, nil
}
