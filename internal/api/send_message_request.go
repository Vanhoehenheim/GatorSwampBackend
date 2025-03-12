package api

type SendMessageRequest struct {
	FromID  string `json:"fromId"`
	ToID    string `json:"toId"`
	Content string `json:"content"`
}
