/*
Sniperkit-Bot
- Status: analyzed
*/

package db

import (
	"context"
	"database/sql"
	"encoding/json"
	"time"

	"github.com/bradleyfalzon/ghfilter"
	"github.com/google/go-github/github"
	"github.com/jmoiron/sqlx"
	"github.com/pkg/errors"
	"golang.org/x/oauth2"
)

// DB represents a database.
type DB interface {
	// Users returns a list of active users that need are scheduled to be polled.
	Users(context.Context) ([]User, error)
	// User returns a single user from the database, returns nil if no user was found.
	User(ctx context.Context, userID int) (*User, error)
	// UserUpdate updates a user in the database.
	UserUpdate(context.Context, *User) error
	// UsersFilters returns all filters for a User ID.
	UsersFilters(ctx context.Context, userID int) ([]Filter, error)
	// Filter returns a single filter from the database, returns nil if no filter found.
	Filter(ctx context.Context, filterID int) (*Filter, error)
	// FilterUpdate updates a filter in the database.
	FilterUpdate(context.Context, *Filter) error
	// Condition returns a single condition from the database, returns nil if no condition found.
	Condition(ctx context.Context, conditionID int) (*Condition, error)
	// ConditionDelete deletes a userID's condition from the database.
	ConditionDelete(ctsx context.Context, userID, conditionID int) error
	// ConditionCreate inserts a condition into the database.
	ConditionCreate(context.Context, *Condition) (conditionID int, err error)
	// SetUsersNextUpdate
	SetUsersPollResult(ctx context.Context, userID int, lastCreatedAt time.Time, nextUpdate time.Time) error
	// GitHubLogin logs a user in via GitHub, if a user already exists with the same
	// githubID, the user's accessToken is updated, else a new user is created.
	GitHubLogin(ctx context.Context, email string, githubID int, githubLogin string, token *oauth2.Token) (userID int, err error)
}

type Dates struct {
	CreatedAt time.Time `db:"created_at"`
	UpdatedAt time.Time `db:"updated_at"`
}

type User struct {
	Dates

	ID int `db:"id"`

	Email          string `db:"email"`
	GitHubID       int    `db:"github_id"`
	GitHubLogin    string `db:"github_login"`
	GitHubTokenRaw []byte `db:"github_token"`
	GitHubToken    *oauth2.Token

	FilterDefaultDiscard bool `db:"filter_default_discard"`

	EventLastCreatedAt time.Time // the latest created at event for the customer
	EventNextPoll      time.Time // time when the next update should occur
}

// Filter represents a single filter from the filters table.
type Filter struct {
	Dates
	ID     int `db:"id"`
	UserID int `db:"user_id"`
	// If discard is true, the filter matching causes an event to be discarded
	// instead of accepted.
	OnMatchDiscard bool `db:"on_match_discard"`

	Conditions []Condition
}

// ghfilter returns a ghfilter.Filter.
func (f *Filter) ghfilter() ghfilter.Filter {
	var ghf ghfilter.Filter
	for _, c := range f.Conditions {
		ghf.Conditions = append(ghf.Conditions, c.GHCondition())
	}
	return ghf
}

// Matches if filter matches an event.
func (f *Filter) Matches(event *github.Event) bool {
	ghf := f.ghfilter()
	return ghf.Matches(event)
}

// Condition represents a single condition from the conditions table.
type Condition struct {
	Dates

	ID                         int    `db:"id"`
	FilterID                   int    `db:"filter_id"`
	Negate                     bool   `db:"negate"`
	Type                       string `db:"type"`
	PayloadAction              string `db:"payload_action"`
	PayloadIssueLabel          string `db:"payload_issue_label"`
	PayloadIssueMilestoneTitle string `db:"payload_issue_milestone_title"`
	PayloadIssueTitleRegexp    string `db:"payload_issue_title_regexp"`
	PayloadIssueBodyRegexp     string `db:"payload_issue_body_regexp"`
	ComparePublic              bool   `db:"compare_public"`
	Public                     bool   `db:"public"`
	OrganizationID             int    `db:"organization_id"`
	RepositoryID               int    `db:"repository_id"`
}

// Condition and Filter should embed the other type.
// and the SQLDB should create its own type to select out of the DB

func (c Condition) GHCondition() ghfilter.Condition {
	return ghfilter.Condition{
		Negate:                     c.Negate,
		Type:                       c.Type,
		PayloadAction:              c.PayloadAction,
		PayloadIssueLabel:          c.PayloadIssueLabel,
		PayloadIssueMilestoneTitle: c.PayloadIssueMilestoneTitle,
		PayloadIssueTitleRegexp:    c.PayloadIssueTitleRegexp,
		PayloadIssueBodyRegexp:     c.PayloadIssueBodyRegexp,
		ComparePublic:              c.ComparePublic,
		Public:                     c.Public,
		OrganizationID:             c.OrganizationID,
		RepositoryID:               c.RepositoryID,
	}
}

func (c Condition) String() string {
	return c.GHCondition().String()
}

type SQLDB struct {
	sqlx *sqlx.DB
}

var _ DB = &SQLDB{}

func NewSQLDB(driver string, dbConn *sql.DB) *SQLDB {
	return &SQLDB{
		sqlx: sqlx.NewDb(dbConn, driver),
	}
}

// Users implements the DB interface.
func (db *SQLDB) Users(_ context.Context) ([]User, error) {
	// TODO only select users where next poll is before now.
	return []User{
		{
			ID:       1,
			Email:    "",
			GitHubID: 1,
			//GitHubToken: []
			GitHubLogin:        "bradleyfalzon",
			EventLastCreatedAt: time.Date(2017, 07, 03, 0, 0, 0, 0, time.UTC),
		},
	}, nil
}

func (db *SQLDB) User(ctx context.Context, userID int) (*User, error) {
	user := &User{}
	err := db.sqlx.GetContext(ctx, user, "SELECT id, email, github_id, github_login, github_token, filter_default_discard FROM users WHERE id = ?", userID)
	switch {
	case err == sql.ErrNoRows:
		return nil, nil
	case err != nil:
		return nil, errors.Wrap(err, "could not select from users")
	}

	if err := json.Unmarshal(user.GitHubTokenRaw, &user.GitHubToken); err != nil {
		return nil, errors.Wrapf(err, "could not unmarshal github token %q", user.GitHubTokenRaw)
	}

	return user, nil
}

// UserUpdate implements the DB interface.
func (db *SQLDB) UserUpdate(ctx context.Context, user *User) error {
	_, err := db.sqlx.ExecContext(ctx, "UPDATE users SET filter_default_discard = ? WHERE id = ?", user.FilterDefaultDiscard, user.ID)
	return errors.Wrapf(err, "could update user %d", user.ID)
}

// UsersFilters implements the DB interface.
func (db *SQLDB) UsersFilters(ctx context.Context, userID int) ([]Filter, error) {
	var filters []Filter
	err := db.sqlx.SelectContext(ctx, &filters, `SELECT id, user_id, on_match_discard, created_at, updated_at FROM filters WHERE user_id = ?`, userID)
	switch {
	case err == sql.ErrNoRows:
		return nil, nil
	case err != nil:
		return nil, errors.Wrap(err, "could not select from filters")
	}

	// I feel terrible that I've written this. Let's hope no-one else uses this service.
	for i := range filters {
		err = db.sqlx.SelectContext(ctx, &filters[i].Conditions, `SELECT * FROM conditions WHERE filter_id = ?`, filters[i].ID)
		switch {
		case err == sql.ErrNoRows:
		case err != nil:
			return nil, errors.Wrap(err, "could not select from conditions")
		}
	}

	return filters, nil
}

// Filter implements the DB interface.
func (db *SQLDB) Filter(ctx context.Context, filterID int) (*Filter, error) {
	filter := &Filter{}
	err := db.sqlx.GetContext(ctx, filter, `SELECT id, user_id, on_match_discard, created_at, updated_at FROM filters WHERE id = ?`, filterID)
	switch {
	case err == sql.ErrNoRows:
		return nil, nil
	case err != nil:
		return nil, errors.Wrap(err, "could not select from filters")
	}

	err = db.sqlx.SelectContext(ctx, &filter.Conditions, `SELECT * FROM conditions WHERE filter_id = ?`, filterID)
	switch {
	case err == sql.ErrNoRows:
	case err != nil:
		return nil, errors.Wrap(err, "could not select from conditions")
	}

	return filter, nil
}

// FilterUpdate implements the DB interface.
func (db *SQLDB) FilterUpdate(ctx context.Context, filter *Filter) error {
	_, err := db.sqlx.ExecContext(ctx, "UPDATE filters SET on_match_discard = ? WHERE id = ?", filter.OnMatchDiscard, filter.ID)
	return errors.Wrapf(err, "could update filter %d", filter.ID)
}

// Condition implements the DB interface.
func (db *SQLDB) Condition(ctx context.Context, conditionID int) (*Condition, error) {
	condition := &Condition{}
	err := db.sqlx.GetContext(ctx, condition, `SELECT * FROM conditions WHERE id = ?`, conditionID)
	switch {
	case err == sql.ErrNoRows:
		return nil, nil
	case err != nil:
		return nil, errors.Wrap(err, "could not select from conditions")
	}
	return condition, nil
}

// ConditionDelete implements the DB interface.
func (db *SQLDB) ConditionDelete(ctx context.Context, userID, conditionID int) error {
	_, err := db.sqlx.ExecContext(ctx, `DELETE c FROM conditions c JOIN filters f ON c.filter_id = f.id WHERE f.user_id = ? AND c.id = ?`, userID, conditionID)
	return errors.Wrap(err, "could not delete condition")
}

// ConditionCreate implements the DB interface.
func (db *SQLDB) ConditionCreate(ctx context.Context, condition *Condition) (int, error) {
	result, err := db.sqlx.NamedExecContext(ctx, `
INSERT INTO conditions (
	filter_id, negate, type, payload_action, payload_issue_label, payload_issue_milestone_title, payload_issue_title_regexp,
	payload_issue_body_regexp, public, organization_id, repository_id
) VALUES (
	:filter_id, :negate, :type, :payload_action, :payload_issue_label, :payload_issue_milestone_title, :payload_issue_title_regexp,
	:payload_issue_body_regexp, :public, :organization_id, :repository_id
)`, condition)
	if err != nil {
		return 0, errors.Wrap(err, "could not insert condition")
	}

	conditionID, err := result.LastInsertId()
	if err != nil {
		return 0, errors.Wrap(err, "could not get condition's ID")
	}

	return int(conditionID), nil
}

// SetUsersPollResult implements the DB interface.
func (db *SQLDB) SetUsersPollResult(ctx context.Context, userID int, lastCreatedAt, nextPoll time.Time) error {
	// TODO do
	return nil
}

// GitHubLogin implements the DB interface.
func (db *SQLDB) GitHubLogin(ctx context.Context, email string, githubID int, githubLogin string, token *oauth2.Token) (int, error) {
	jsonToken, err := json.Marshal(token)
	if err != nil {
		return 0, errors.Wrap(err, "could not marshal oauth2.token")
	}

	// Check if user exists
	var userID int
	err = db.sqlx.QueryRowContext(ctx, "SELECT id FROM users WHERE github_id = ?", githubID).Scan(&userID)
	switch {
	case err == sql.ErrNoRows:
		// Add token to new user
		res, err := db.sqlx.ExecContext(ctx, "INSERT INTO users (email, github_id, github_login, github_token) VALUES (?, ?, ?, ?)", email, githubID, githubLogin, jsonToken)
		if err != nil {
			return 0, errors.Wrapf(err, "error inserting new githubID %q", githubID)
		}
		id, err := res.LastInsertId()
		if err != nil {
			return 0, errors.Wrap(err, "error in lastInsertId")
		}
		return int(id), nil
	case err != nil:
		return 0, errors.Wrapf(err, "error getting userID for githubID %q", githubID)
	}

	// Add token to existing user and update email
	_, err = db.sqlx.ExecContext(ctx, "UPDATE users SET email = ?, github_login = ?, github_token = ? WHERE id = ?", email, githubLogin, jsonToken, userID)
	if err != nil {
		return 0, errors.Wrapf(err, "could update userID %d", userID)
	}
	return userID, nil
}
