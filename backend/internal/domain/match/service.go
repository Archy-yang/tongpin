package match

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
)

// UserDirectory 本域对用户基础信息的最小依赖（消费方接口，装配层注入）。
type UserDirectory interface {
	Basic(ctx context.Context, userID int64) (BasicUser, error)
}

type BasicUser struct {
	Nickname   string
	Gender     string
	GenderPref string
	AgeRange   string
	City       string
}

// BlockChecker 本域对安全域的最小依赖。
type BlockChecker interface {
	BlockedBetween(ctx context.Context, a, b int64) (bool, error)
}

// ConsentRecorder 本域对授权审计的最小依赖。
type ConsentRecorder interface {
	Record(ctx context.Context, userID int64, targetType string, targetID *int64, granted bool) error
}

const (
	// maxSuggestions 每次最多持有的契合提示数（宁缺毋滥）
	maxSuggestions = 3
	// defaultPoolThreshold 匹配池低于该人数时契合提示灰度关闭（冷启动保护）
	defaultPoolThreshold = 500
)

type Service struct {
	repo          *Repository
	users         UserDirectory
	blocks        BlockChecker
	consents      ConsentRecorder
	poolThreshold int
}

func NewService(repo *Repository, users UserDirectory, blocks BlockChecker, consents ConsentRecorder, poolThreshold int) *Service {
	if poolThreshold < 0 { // 0 = 不设阈值（测试模式），负数才回退默认
		poolThreshold = defaultPoolThreshold
	}
	return &Service{repo: repo, users: users, blocks: blocks, consents: consents, poolThreshold: poolThreshold}
}

// OptIn 进入匹配池。要求风格画像已生成（向量可用）。
func (s *Service) OptIn(ctx context.Context, userID int64, req OptInRequest) (OptInView, error) {
	if !validIntents[req.Intent] {
		return OptInView{}, errInvalidOptIn
	}
	if req.AgePrefMin != nil && req.AgePrefMax != nil && *req.AgePrefMin > *req.AgePrefMax {
		return OptInView{}, errInvalidOptIn
	}
	accept := true
	if req.AcceptRealContact != nil {
		accept = *req.AcceptRealContact
	}
	me, err := s.repo.MyProfile(ctx, userID)
	if err != nil {
		return OptInView{}, fmt.Errorf("load profile: %w", err)
	}
	if me == nil {
		return OptInView{}, ErrProfileNotReady // 画像未就绪不允许进入匹配池
	}

	o := OptIn{UserID: userID, Intent: req.Intent, AgePrefMin: req.AgePrefMin, AgePrefMax: req.AgePrefMax, AcceptRealContact: accept, Status: "active"}
	if err := s.repo.UpsertOptIn(ctx, o); err != nil {
		return OptInView{}, fmt.Errorf("upsert opt-in: %w", err)
	}
	if err := s.consents.Record(ctx, userID, "match_pool", nil, true); err != nil {
		return OptInView{}, fmt.Errorf("record consent: %w", err)
	}
	return o.view(), nil
}

var ErrProfileNotReady = fmt.Errorf("style profile not ready")
var ErrNotOptedIn = fmt.Errorf("not opted in")
var ErrMatchNotFound = fmt.Errorf("match not found")
var ErrNotParticipant = fmt.Errorf("not a participant of this match")

// Withdraw 退出匹配池（撤回匹配授权）。
func (s *Service) Withdraw(ctx context.Context, userID int64) error {
	o, err := s.repo.GetOptIn(ctx, userID)
	if err != nil {
		return err
	}
	if o == nil || o.Status != "active" {
		return ErrNotOptedIn
	}
	if err := s.repo.Withdraw(ctx, userID); err != nil {
		return err
	}
	return s.consents.Record(ctx, userID, "match_pool", nil, false)
}

// MyOptIn 当前 opt-in 状态。
func (s *Service) MyOptIn(ctx context.Context, userID int64) (*OptInView, error) {
	o, err := s.repo.GetOptIn(ctx, userID)
	if err != nil || o == nil {
		return nil, err
	}
	v := o.view()
	return &v, nil
}

// Suggestions 契合提示：池子达到阈值才生成新提示；返回我当前全部 suggested 提示。
func (s *Service) Suggestions(ctx context.Context, userID int64) (SuggestionsView, error) {
	active, err := s.repo.ActivePoolCount(ctx)
	if err != nil {
		return SuggestionsView{}, err
	}
	if active < s.poolThreshold {
		return SuggestionsView{PoolStatus: "warming_up", ActiveUsers: active, Suggestions: []Suggestion{}}, nil
	}

	if err := s.generateIfNeeded(ctx, userID); err != nil {
		return SuggestionsView{}, err
	}

	rows, err := s.repo.MySuggested(ctx, userID)
	if err != nil {
		return SuggestionsView{}, err
	}
	out := make([]Suggestion, 0, len(rows))
	for _, m := range rows {
		var detail ScoreBreakdown
		_ = json.Unmarshal(m.ScoreDetail, &detail)
		out = append(out, Suggestion{
			MatchID: m.ID, Score: m.Score, Highlights: detail.Highlights, CreatedAt: m.CreatedAt,
		})
	}
	return SuggestionsView{PoolStatus: "active", ActiveUsers: active, Suggestions: out}, nil
}

// generateIfNeeded 我持有 < maxSuggestions 条提示时，从候选中打分补足。
func (s *Service) generateIfNeeded(ctx context.Context, userID int64) error {
	existing, err := s.repo.MySuggested(ctx, userID)
	if err != nil {
		return err
	}
	if len(existing) >= maxSuggestions {
		return nil
	}

	me, err := s.repo.MyProfile(ctx, userID)
	if err != nil {
		return err
	}
	if me == nil {
		return ErrProfileNotReady
	}
	myBasic, err := s.users.Basic(ctx, userID)
	if err != nil {
		return err
	}
	meCand := toCandidate(*me, myBasic)
	myProf := parseMatchProfile(me.Profile)
	myVec := parseEmbedding(me.Embedding)

	candidates, err := s.repo.Candidates(ctx, userID)
	if err != nil {
		return err
	}

	type scored struct {
		cand  Candidate
		total float64
		detail ScoreBreakdown
	}
	var pool []scored
	for _, cand := range candidates {
		blocked, err := s.blocks.BlockedBetween(ctx, userID, cand.UserID)
		if err != nil {
			return err
		}
		if blocked {
			continue
		}
		basic, err := s.users.Basic(ctx, cand.UserID)
		if err != nil {
			return err
		}
		if !hardFilter(toCandidate(cand, basic), meCand) {
			continue
		}
		candProf := parseMatchProfile(cand.Profile)
		detail := ScoreBreakdown{
			Cosine:     cosine(myVec, parseEmbedding(cand.Embedding)),
			Intent:     intentScore(myProf.Intent, candProf.Intent),
			Hours:      hoursOverlap(myProf.ActiveHours, candProf.ActiveHours),
			Geo:        geoScore(myBasic.City, basic.City),
			Complement: complementScore(myProf.Initiative, candProf.Initiative),
			Highlights: buildHighlights(myProf, candProf, myBasic.City, basic.City),
		}
		pool = append(pool, scored{cand: cand, total: detail.Total(), detail: detail})
	}

	sort.Slice(pool, func(i, j int) bool { return pool[i].total > pool[j].total })
	for i := 0; i < len(pool) && len(existing)+i < maxSuggestions; i++ {
		p := pool[i]
		a, b := order(userID, p.cand.UserID)
		detailJSON, err := json.Marshal(p.detail)
		if err != nil {
			return err
		}
		if _, err := s.repo.InsertMatch(ctx, a, b, p.total, detailJSON); err != nil {
			return err
		}
	}
	return nil
}

// CloseByUser 供 consent 域通过接口调用：撤回授权时关闭匹配。
func (s *Service) CloseByUser(ctx context.Context, matchID, userID int64) error {
	return s.repo.CloseByUser(ctx, matchID, userID)
}

// ParticipantOf / CounterpartOf / StatusOf / SetStatus 供 contact 域通过接口调用。
func (s *Service) ParticipantOf(ctx context.Context, matchID, userID int64) (bool, error) {
	m, err := s.repo.GetByID(ctx, matchID)
	if err != nil || m == nil {
		return false, err
	}
	return m.UserA == userID || m.UserB == userID, nil
}

func (s *Service) CounterpartOf(ctx context.Context, matchID, userID int64) (int64, error) {
	m, err := s.repo.GetByID(ctx, matchID)
	if err != nil {
		return 0, err
	}
	if m == nil || (m.UserA != userID && m.UserB != userID) {
		return 0, ErrNotParticipant
	}
	if m.UserA == userID {
		return m.UserB, nil
	}
	return m.UserA, nil
}

func (s *Service) StatusOf(ctx context.Context, matchID int64) (string, error) {
	m, err := s.repo.GetByID(ctx, matchID)
	if err != nil {
		return "", err
	}
	if m == nil {
		return "", ErrMatchNotFound
	}
	return m.Status, nil
}

func (s *Service) SetStatus(ctx context.Context, matchID int64, status string) error {
	return s.repo.UpdateStatus(ctx, matchID, status)
}

func geoScore(a, b string) float64 {
	if a != "" && a == b {
		return 1
	}
	return 0
}

func order(a, b int64) (int64, int64) {
	if a < b {
		return a, b
	}
	return b, a
}

// toCandidate 把仓库行与用户目录信息合成打分所需的完整候选。
func toCandidate(c Candidate, basic BasicUser) Candidate {
	if basic.Gender != "" {
		c.Gender = basic.Gender
	}
	if basic.AgeRange != "" {
		c.AgeRange = basic.AgeRange
	}
	if basic.City != "" {
		c.City = basic.City
	}
	return c
}

// parseEmbedding 解析 pgvector ::text 输出 "[0.1,0.2,...]"。
func parseEmbedding(s string) []float64 {
	s = strings.TrimPrefix(strings.TrimSuffix(strings.TrimSpace(s), "]"), "[")
	parts := strings.Split(s, ",")
	out := make([]float64, 0, len(parts))
	for _, p := range parts {
		v, err := strconv.ParseFloat(strings.TrimSpace(p), 64)
		if err != nil {
			continue
		}
		out = append(out, v)
	}
	return out
}

func (o OptIn) view() OptInView {
	return OptInView{Intent: o.Intent, AgePrefMin: o.AgePrefMin, AgePrefMax: o.AgePrefMax,
		AcceptRealContact: o.AcceptRealContact, Status: o.Status}
}
