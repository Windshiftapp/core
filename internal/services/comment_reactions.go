package services

import (
	"database/sql"
	"errors"
	"fmt"
	"slices"
	"sync"
	"time"

	"windshift/internal/models"
	"windshift/internal/repository"
)

// CommentReactionKeys is the fixed set of reactions a user can leave on a
// comment, in display order (core#298). Keys, not emoji, travel over the API
// so clients render them consistently.
var CommentReactionKeys = []string{"thumbs_up", "check", "eyes", "heart", "laugh", "hooray"}

// ErrInvalidCommentReaction reports a reaction key outside CommentReactionKeys.
var ErrInvalidCommentReaction = errors.New("reaction is not supported")

// maxReactionCommentIDs bounds one IN clause; comment feeds return at most
// MaxCommentFeedLimit rows per item, but batches span many items.
const maxReactionCommentIDs = 500

// reactionCacheTTL bounds how long grouped reactions are served from process
// memory without a database read. Writes invalidate the affected comment in
// this process; the TTL only bounds staleness for other processes in a
// multi-instance deployment.
const reactionCacheTTL = 5 * time.Second

// reactionCacheMaxEntries caps memory. Entries expire lazily, so a process can
// hold at most this many between prunes.
const reactionCacheMaxEntries = 20000

type reactionCacheEntry struct {
	groups    []models.CommentReaction
	expiresAt time.Time
}

// commentReactionCache memoizes grouped reactions per comment. The zero value
// is ready to use. Callers must treat the returned groups as read-only.
type commentReactionCache struct {
	mu      sync.Mutex
	entries map[int]reactionCacheEntry
	// now is the clock used for expiry; tests override it. Nil means time.Now.
	now func() time.Time
}

func (c *commentReactionCache) clock() time.Time {
	if c.now != nil {
		return c.now()
	}
	return time.Now()
}

func (c *commentReactionCache) get(commentID int) ([]models.CommentReaction, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	entry, ok := c.entries[commentID]
	if !ok {
		return nil, false
	}
	if c.clock().After(entry.expiresAt) {
		delete(c.entries, commentID)
		return nil, false
	}
	return entry.groups, true
}

func (c *commentReactionCache) set(commentID int, groups []models.CommentReaction) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.entries == nil {
		c.entries = make(map[int]reactionCacheEntry)
	}
	if len(c.entries) >= reactionCacheMaxEntries {
		c.pruneExpiredLocked()
	}
	if len(c.entries) >= reactionCacheMaxEntries {
		clear(c.entries)
	}
	c.entries[commentID] = reactionCacheEntry{groups: groups, expiresAt: c.clock().Add(reactionCacheTTL)}
}

func (c *commentReactionCache) invalidate(commentID int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.entries, commentID)
}

func (c *commentReactionCache) pruneExpiredLocked() {
	now := c.clock()
	for id, entry := range c.entries {
		if now.After(entry.expiresAt) {
			delete(c.entries, id)
		}
	}
}

// ValidCommentReaction reports whether key is a supported reaction.
func ValidCommentReaction(key string) bool {
	return slices.Contains(CommentReactionKeys, key)
}

// AddReaction records userID's reaction on a comment. Adding the same
// reaction twice is a no-op.
func (s *CommentService) AddReaction(commentID, userID int, reaction string) ([]models.CommentReaction, error) {
	if !ValidCommentReaction(reaction) {
		return nil, ErrInvalidCommentReaction
	}
	itemID, err := s.reactionCommentItem(commentID)
	if err != nil {
		return nil, err
	}
	if _, err := s.db.ExecWrite(`
		INSERT INTO comment_reactions (comment_id, user_id, reaction) VALUES (?, ?, ?)
		ON CONFLICT (comment_id, user_id, reaction) DO NOTHING`, commentID, userID, reaction); err != nil {
		return nil, fmt.Errorf("add reaction to comment %d: %w", commentID, err)
	}
	s.reactionCache.invalidate(commentID)
	PublishItemChange(itemID)
	return s.ReactionsForComment(commentID)
}

// RemoveReaction withdraws userID's reaction from a comment. Removing a
// reaction that does not exist is a no-op.
func (s *CommentService) RemoveReaction(commentID, userID int, reaction string) error {
	if !ValidCommentReaction(reaction) {
		return ErrInvalidCommentReaction
	}
	itemID, err := s.reactionCommentItem(commentID)
	if err != nil {
		return err
	}
	if _, err := s.db.ExecWrite(`DELETE FROM comment_reactions WHERE comment_id = ? AND user_id = ? AND reaction = ?`,
		commentID, userID, reaction); err != nil {
		return fmt.Errorf("remove reaction from comment %d: %w", commentID, err)
	}
	s.reactionCache.invalidate(commentID)
	PublishItemChange(itemID)
	return nil
}

// ReactionsForComment returns the grouped reactions on one comment.
func (s *CommentService) ReactionsForComment(commentID int) ([]models.CommentReaction, error) {
	byComment, err := s.reactionsByComment([]int{commentID})
	if err != nil {
		return nil, err
	}
	reactions := byComment[commentID]
	if reactions == nil {
		reactions = []models.CommentReaction{}
	}
	return reactions, nil
}

// AttachReactions fills Reactions on the human comments in place. Approval
// rows (non-positive IDs) are skipped.
func (s *CommentService) AttachReactions(comments []models.Comment) error {
	ids := make([]int, 0, len(comments))
	for _, comment := range comments {
		if comment.ID > 0 && comment.Source != "approval" {
			ids = append(ids, comment.ID)
		}
	}
	byComment, err := s.reactionsByComment(ids)
	if err != nil {
		return err
	}
	for i := range comments {
		comments[i].Reactions = byComment[comments[i].ID]
	}
	return nil
}

func (s *CommentService) reactionCommentItem(commentID int) (int, error) {
	var itemID int
	err := s.db.QueryRow(`SELECT item_id FROM comments WHERE id = ?`, commentID).Scan(&itemID)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, fmt.Errorf("comment %d: %w", commentID, repository.ErrNotFound)
	}
	if err != nil {
		return 0, fmt.Errorf("find comment %d: %w", commentID, err)
	}
	return itemID, nil
}

func (s *CommentService) reactionsByComment(ids []int) (map[int][]models.CommentReaction, error) {
	result := make(map[int][]models.CommentReaction, len(ids))
	missing := make([]int, 0, len(ids))
	seen := make(map[int]struct{}, len(ids))
	for _, id := range ids {
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		if groups, ok := s.reactionCache.get(id); ok {
			result[id] = groups
			continue
		}
		missing = append(missing, id)
	}
	for start := 0; start < len(missing); start += maxReactionCommentIDs {
		chunk := missing[start:min(start+maxReactionCommentIDs, len(missing))]
		args := make([]any, len(chunk))
		for i, id := range chunk {
			args[i] = id
		}
		rows, err := s.db.Query(fmt.Sprintf(`
			SELECT r.comment_id, r.reaction, r.user_id,
			       COALESCE(NULLIF(TRIM(COALESCE(u.first_name, '') || ' ' || COALESCE(u.last_name, '')), ''), u.username, 'Unknown User')
			FROM comment_reactions r
			JOIN users u ON u.id = r.user_id
			WHERE r.comment_id IN (%s)
			ORDER BY r.comment_id, r.created_at, r.id`, placeholders(len(chunk))), args...)
		if err != nil {
			return nil, fmt.Errorf("list comment reactions: %w", err)
		}
		found := make(map[int][]models.CommentReaction, len(chunk))
		for rows.Next() {
			var commentID, userID int
			var reaction, name string
			if err := rows.Scan(&commentID, &reaction, &userID, &name); err != nil {
				_ = rows.Close()
				return nil, fmt.Errorf("scan comment reaction: %w", err)
			}
			found[commentID] = appendReaction(found[commentID], reaction, userID, name)
		}
		err = rows.Err()
		_ = rows.Close()
		if err != nil {
			return nil, fmt.Errorf("read comment reactions: %w", err)
		}
		for _, id := range chunk {
			groups := found[id]
			if groups == nil {
				groups = []models.CommentReaction{}
			}
			sortReactionGroups(groups)
			s.reactionCache.set(id, groups)
			result[id] = groups
		}
	}
	return result, nil
}

func sortReactionGroups(groups []models.CommentReaction) {
	slices.SortStableFunc(groups, func(a, b models.CommentReaction) int {
		return slices.Index(CommentReactionKeys, a.Reaction) - slices.Index(CommentReactionKeys, b.Reaction)
	})
}

func appendReaction(groups []models.CommentReaction, reaction string, userID int, name string) []models.CommentReaction {
	for i := range groups {
		if groups[i].Reaction == reaction {
			groups[i].Count++
			groups[i].UserIDs = append(groups[i].UserIDs, userID)
			groups[i].UserNames = append(groups[i].UserNames, name)
			return groups
		}
	}
	return append(groups, models.CommentReaction{Reaction: reaction, Count: 1, UserIDs: []int{userID}, UserNames: []string{name}})
}
