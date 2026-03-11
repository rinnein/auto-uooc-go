package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

const baseURL = "https://www.uooc.net.cn"

type Client struct {
	httpClient *http.Client
	cookie     string
}

func NewClient(cookie string) *Client {
	return &Client{
		httpClient: &http.Client{},
		cookie:     strings.TrimSpace(cookie),
	}
}

type envelope[T any] struct {
	Code int    `json:"code"`
	Msg  string `json:"msg"`
	Data T      `json:"data"`
}

type User struct {
	ID   int64  `json:"id"`
	Nick string `json:"nick"`
	Name string `json:"name"`
}

type CatalogNode struct {
	ID       int64         `json:"id"`
	Name     string        `json:"name"`
	Parent   int64         `json:"parent"`
	Children []CatalogNode `json:"children"`
}

type CourseProgress struct {
	VideoProgress      string `json:"video_progress"`
	VideoProgressValue int    `json:"video_progress_value"`
}

type UnitItem struct {
	ID            int64                  `json:"id"`
	Title         string                 `json:"title"`
	Finished      int                    `json:"finished"`
	VideoURL      map[string]VideoSource `json:"video_url"`
	VideoPlayList []VideoSource          `json:"video_play_list"`
}

type VideoSource struct {
	Source string `json:"source"`
}

type MarkVideoResp struct {
	Finished int `json:"finished"`
}

type APIError struct {
	Code int
	Msg  string
}

func (e *APIError) Error() string {
	return fmt.Sprintf("api error: code=%d msg=%s", e.Code, e.Msg)
}

func IsAPIErrorCode(err error, code int) bool {
	var apiErr *APIError
	return errors.As(err, &apiErr) && apiErr.Code == code
}

func (c *Client) GetUser(ctx context.Context) (User, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, baseURL+"/home/member/user", nil)
	if err != nil {
		return User{}, err
	}
	var out envelope[User]
	if err := c.doJSON(req, &out); err != nil {
		return User{}, err
	}
	if out.Code != 1 || out.Data.ID == 0 {
		return User{}, &APIError{Code: out.Code, Msg: out.Msg}
	}
	return out.Data, nil
}

func (c *Client) GetCatalogList(ctx context.Context, cid int64) ([]CatalogNode, error) {
	u := fmt.Sprintf("%s/home/learn/getCatalogList?cid=%d&hidemsg_=true", baseURL, cid)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	var out envelope[[]CatalogNode]
	if err := c.doJSON(req, &out); err != nil {
		return nil, err
	}
	if out.Code != 1 {
		return nil, &APIError{Code: out.Code, Msg: out.Msg}
	}
	return out.Data, nil
}

func (c *Client) GetCourseProgress(ctx context.Context, cid int64) (CourseProgress, error) {
	u := fmt.Sprintf("%s/home/course/progress?cid=%d", baseURL, cid)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return CourseProgress{}, err
	}
	var out envelope[CourseProgress]
	if err := c.doJSON(req, &out); err != nil {
		return CourseProgress{}, err
	}
	if out.Code != 1 {
		return CourseProgress{}, &APIError{Code: out.Code, Msg: out.Msg}
	}
	return out.Data, nil
}

func (c *Client) GetUnitLearn(ctx context.Context, cid, chapterID, sectionID int64) ([]UnitItem, error) {
	q := url.Values{}
	q.Set("catalog_id", strconv.FormatInt(sectionID, 10))
	q.Set("chapter_id", strconv.FormatInt(chapterID, 10))
	q.Set("cid", strconv.FormatInt(cid, 10))
	q.Set("hidemsg_", "true")
	q.Set("section_id", strconv.FormatInt(sectionID, 10))
	u := baseURL + "/home/learn/getUnitLearn?" + q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	var out envelope[[]UnitItem]
	if err := c.doJSON(req, &out); err != nil {
		return nil, err
	}
	if out.Code != 1 {
		return nil, &APIError{Code: out.Code, Msg: out.Msg}
	}
	return out.Data, nil
}

func (c *Client) MarkVideoLearn(ctx context.Context, req MarkVideoRequest) (MarkVideoResp, error) {
	form := url.Values{}
	form.Set("chapter_id", strconv.FormatInt(req.ChapterID, 10))
	form.Set("cid", strconv.FormatInt(req.CID, 10))
	form.Set("hidemsg_", "true")
	form.Set("network", "2")
	form.Set("resource_id", strconv.FormatInt(req.ResourceID, 10))
	form.Set("section_id", strconv.FormatInt(req.SectionID, 10))
	form.Set("source", "1")
	form.Set("subsection_id", "0")
	form.Set("video_length", fmt.Sprintf("%.2f", req.VideoLength))
	form.Set("video_pos", fmt.Sprintf("%.2f", req.VideoPos))

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL+"/home/learn/markVideoLearn", strings.NewReader(form.Encode()))
	if err != nil {
		return MarkVideoResp{}, err
	}
	httpReq.Header.Set("Content-Type", "application/x-www-form-urlencoded;charset=UTF-8")

	var out envelope[MarkVideoResp]
	if err := c.doJSON(httpReq, &out); err != nil {
		return MarkVideoResp{}, err
	}
	if out.Code != 1 {
		return MarkVideoResp{}, &APIError{Code: out.Code, Msg: out.Msg}
	}
	return out.Data, nil
}

type MarkVideoRequest struct {
	ChapterID   int64
	CID         int64
	ResourceID  int64
	SectionID   int64
	VideoLength float64
	VideoPos    float64
}

func (c *Client) doJSON(req *http.Request, out any) error {
	req.Header.Set("Accept", "application/json, text/plain, */*")
	req.Header.Set("Cookie", c.cookie)
	req.Header.Set("Referer", baseURL+"/home")
	req.Header.Set("XSRF", "")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("http status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return fmt.Errorf("decode response: %w", err)
	}
	return nil
}
