package routes

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/vango-go/vango"
	. "github.com/vango-go/vango/el"
	"github.com/vango-go/vango/setup"

	chatsvc "rhone_chat/internal/services/chat"
)

type ToolCallView struct {
	ID      string
	Name    string
	Status  string
	Input   string
	Output  string
	ErrText string
}

type MessageView struct {
	ID        string
	Role      string
	Content   string
	Status    string
	ToolCalls []ToolCallView
	CreatedAt time.Time
}

type PendingRun struct {
	RunID              string
	ChatID             string
	UserMessageID      string
	AssistantMessageID string
	Model              string
	UserContent        string
}

type renameChatRequest struct {
	ChatID string
	Title  string
}

type runExecution struct {
	RunID              string
	AssistantMessageID string
	Status             string
	ErrText            string
}

type themePalette struct {
	AppRoot          string
	Sidebar          string
	SidebarSection   string
	NewChatButton    string
	ChatButtonBase   string
	ChatButtonIdle   string
	ChatButtonActive string
	ChatActionButton string
	ChatDangerButton string
	ChatInput        string
	ChatSaveButton   string
	ChatMeta         string
	Header           string
	HeaderTitle      string
	ModelSelect      string
	ThemeToggle      string
	StopButton       string
	ErrorText        string
	ChatBody         string
	AssistantBubble  string
	UserBubble       string
	ThinkingText     string
	StatusText       string
	RoleText         string
	ToolCard         string
	ToolText         string
	ToolErrorText    string
	Composer         string
	Input            string
	SendButton       string
}

func IndexPage(ctx vango.Ctx) *vango.VNode {
	return Div(ChatRoot(vango.NoProps{}))
}

func ChatRoot(props vango.NoProps) vango.Component {
	return vango.Setup(props, func(s vango.SetupCtx[vango.NoProps]) vango.RenderFn {
		dependencies := getDeps()
		chatService := dependencies.Chat
		sessionCtx := s.Ctx()

		chats := setup.Signal(&s, []chatsvc.Chat{})
		messages := setup.Signal(&s, []MessageView{})
		activeChatID := setup.Signal(&s, "")
		inputText := setup.Signal(&s, "")
		selectedModel := setup.Signal(&s, chatService.DefaultModel())
		errorText := setup.Signal(&s, "")
		isThinking := setup.Signal(&s, false)
		activeRunID := setup.Signal(&s, "")
		activeAssistantID := setup.Signal(&s, "")
		themeMode := setup.Signal(&s, "dark")
		editingChatID := setup.Signal(&s, "")
		renameTitle := setup.Signal(&s, "")

		runTrigger := setup.Signal(&s, 0)
		pendingRun := setup.Signal(&s, PendingRun{})

		loadChatsAction := setup.Action(&s,
			func(workCtx context.Context, _ struct{}) ([]chatsvc.Chat, error) {
				return chatService.ListOrCreateChats(workCtx, 200)
			},
			vango.DropWhileRunning(),
			vango.ActionOnSuccess(func(value any) {
				chatList, ok := value.([]chatsvc.Chat)
				if !ok {
					return
				}
				chats.Set(chatList)
				currentActive := activeChatID.Get()
				if currentActive == "" || !containsChat(chatList, currentActive) {
					currentActive = chatList[0].ID
					activeChatID.Set(currentActive)
				}
				selected := findChatByID(chatList, currentActive)
				if selected.ID != "" && chatService.IsAllowedModel(selected.Model) {
					selectedModel.Set(selected.Model)
				}
				errorText.Set("")
			}),
			vango.ActionOnError(func(err error) {
				errorText.Set(err.Error())
			}),
		)

		loadMessagesAction := setup.Action(&s,
			func(workCtx context.Context, chatID string) ([]chatsvc.Message, error) {
				return chatService.ListMessages(workCtx, chatID, 500)
			},
			vango.CancelLatest(),
			vango.ActionOnSuccess(func(value any) {
				rows, ok := value.([]chatsvc.Message)
				if !ok {
					messages.Set([]MessageView{})
					return
				}
				viewMessages := make([]MessageView, 0, len(rows))
				for _, row := range rows {
					viewMessages = append(viewMessages, MessageView{
						ID:        row.ID,
						Role:      row.Role,
						Content:   row.Content,
						Status:    row.Status,
						CreatedAt: row.CreatedAt,
					})
				}
				messages.Set(viewMessages)
				errorText.Set("")
			}),
			vango.ActionOnError(func(err error) {
				errorText.Set(err.Error())
			}),
		)

		createChatAction := setup.Action(&s,
			func(workCtx context.Context, model string) (chatsvc.Chat, error) {
				return chatService.CreateChat(workCtx, model)
			},
			vango.DropWhileRunning(),
			vango.ActionOnSuccess(func(value any) {
				chat, ok := value.(chatsvc.Chat)
				if !ok {
					return
				}
				current := chats.Get()
				next := make([]chatsvc.Chat, 0, len(current)+1)
				next = append(next, chat)
				next = append(next, current...)
				chats.Set(next)
				activeChatID.Set(chat.ID)
				selectedModel.Set(chat.Model)
				messages.Set([]MessageView{})
				errorText.Set("")
			}),
			vango.ActionOnError(func(err error) {
				errorText.Set(err.Error())
			}),
		)

		renameChatAction := setup.Action(&s,
			func(workCtx context.Context, request renameChatRequest) (string, error) {
				if err := chatService.RenameChat(workCtx, request.ChatID, request.Title); err != nil {
					return "", err
				}
				return strings.TrimSpace(request.Title), nil
			},
			vango.DropWhileRunning(),
			vango.ActionOnSuccess(func(value any) {
				updatedTitle, ok := value.(string)
				if !ok {
					return
				}
				chatID := editingChatID.Get()
				if chatID == "" {
					return
				}
				chats.Set(updateChatTitle(chats.Get(), chatID, updatedTitle))
				editingChatID.Set("")
				renameTitle.Set("")
				errorText.Set("")
			}),
			vango.ActionOnError(func(err error) {
				errorText.Set(err.Error())
			}),
		)

		deleteChatAction := setup.Action(&s,
			func(workCtx context.Context, chatID string) (string, error) {
				if err := chatService.DeleteChat(workCtx, chatID); err != nil {
					return "", err
				}
				return chatID, nil
			},
			vango.DropWhileRunning(),
			vango.ActionOnSuccess(func(value any) {
				deletedChatID, ok := value.(string)
				if !ok {
					return
				}
				currentChats := removeChatByID(chats.Get(), deletedChatID)
				chats.Set(currentChats)
				if editingChatID.Get() == deletedChatID {
					editingChatID.Set("")
					renameTitle.Set("")
				}
				if activeChatID.Get() == deletedChatID {
					if len(currentChats) > 0 {
						activeChatID.Set(currentChats[0].ID)
						if chatService.IsAllowedModel(currentChats[0].Model) {
							selectedModel.Set(currentChats[0].Model)
						}
					} else {
						activeChatID.Set("")
						messages.Set([]MessageView{})
						createChatAction.Run(selectedModel.Get())
					}
				}
				errorText.Set("")
			}),
			vango.ActionOnError(func(err error) {
				errorText.Set(err.Error())
			}),
		)

		s.OnMount(func() vango.Cleanup {
			loadChatsAction.Run(struct{}{})
			return nil
		})

		s.Effect(func() vango.Cleanup {
			chatID := activeChatID.Get()
			if chatID == "" {
				messages.Set([]MessageView{})
				return nil
			}
			loadMessagesAction.Run(chatID)
			return nil
		})

		s.Effect(func() vango.Cleanup {
			trigger := runTrigger.Get()
			if trigger == 0 {
				return nil
			}
			run := pendingRun.Get()
			if run.RunID == "" {
				return nil
			}

			return vango.GoLatest(trigger,
				func(workCtx context.Context, _ int) (runExecution, error) {
					if err := chatService.PersistRunStart(workCtx, chatsvc.PendingRun{
						RunID:              run.RunID,
						ChatID:             run.ChatID,
						UserMessageID:      run.UserMessageID,
						AssistantMessageID: run.AssistantMessageID,
						Model:              run.Model,
					}, run.UserContent); err != nil {
						return runExecution{}, err
					}

					history, err := chatService.BuildHistory(workCtx, run.ChatID)
					if err != nil {
						return runExecution{}, err
					}

					uiFlushInterval, uiFlushBytes, dbFlushInterval := chatService.FlushConfig()
					var assistantBuilder strings.Builder
					pendingDelta := ""
					lastUIFlush := time.Now().UTC()
					lastDBFlush := time.Now().UTC()
					toolCallRowByExternalID := map[string]string{}

					flushUI := func(force bool) {
						if pendingDelta == "" {
							return
						}
						if !force && len(pendingDelta) < uiFlushBytes && time.Since(lastUIFlush) < uiFlushInterval {
							return
						}
						chunk := pendingDelta
						pendingDelta = ""
						assistantBuilder.WriteString(chunk)
						lastUIFlush = time.Now().UTC()
						sessionCtx.Dispatch(func() {
							if activeRunID.Get() != run.RunID {
								return
							}
							messages.Set(appendAssistantChunk(messages.Peek(), run.AssistantMessageID, chunk))
							isThinking.Set(false)
						})
					}

					flushDB := func(force bool) {
						if !force && time.Since(lastDBFlush) < dbFlushInterval {
							return
						}
						lastDBFlush = time.Now().UTC()
						content := assistantBuilder.String() + pendingDelta
						_ = chatService.UpdateAssistantPartial(workCtx, run.AssistantMessageID, content)
					}

					streamResult, streamErr := chatService.Stream(workCtx, run.Model, history, chatsvc.StreamCallbacks{
						OnTextDelta: func(delta string) {
							pendingDelta += delta
							flushUI(false)
							flushDB(false)
						},
						OnThinking: func() {
							sessionCtx.Dispatch(func() {
								if activeRunID.Get() == run.RunID {
									isThinking.Set(true)
								}
							})
						},
						OnToolStart: func(update chatsvc.ToolCallUpdate) {
							flushUI(true)
							callID, callErr := chatService.UpsertToolStart(workCtx, run.RunID, update)
							if callErr == nil && update.ID != "" {
								toolCallRowByExternalID[update.ID] = callID
							}
							sessionCtx.Dispatch(func() {
								if activeRunID.Get() != run.RunID {
									return
								}
								messages.Set(addToolCall(messages.Peek(), run.AssistantMessageID, ToolCallView{
									ID:     callID,
									Name:   update.Name,
									Status: "running",
									Input:  truncateText(update.Input, 500),
								}))
							})
						},
						OnToolResult: func(update chatsvc.ToolCallUpdate) {
							flushUI(true)
							callID := toolCallRowByExternalID[update.ID]
							if callID == "" {
								callID = uuid.NewString()
							}
							_ = chatService.CompleteTool(workCtx, callID, update)
							sessionCtx.Dispatch(func() {
								if activeRunID.Get() != run.RunID {
									return
								}
								messages.Set(updateToolCall(messages.Peek(), run.AssistantMessageID, callID, update.Status, truncateText(update.Output, 500), truncateText(update.ErrText, 300)))
							})
						},
					})

					flushUI(true)
					flushDB(true)
					finalContent := assistantBuilder.String() + pendingDelta

					status := "completed"
					streamErrorText := ""
					if streamErr != nil {
						if chatService.IsCancellation(streamErr, workCtx) {
							status = "cancelled"
						} else {
							status = "error"
							streamErrorText = streamErr.Error()
						}
					}
					if status == "error" && strings.TrimSpace(streamErrorText) == "" {
						streamErrorText = fmt.Sprintf("Model %s failed without a provider error message.", run.Model)
					}

					if err := chatService.CompleteAssistant(workCtx, run.AssistantMessageID, finalContent, status); err != nil {
						return runExecution{}, err
					}
					if err := chatService.CompleteRun(workCtx, chatsvc.PendingRun{
						RunID:              run.RunID,
						ChatID:             run.ChatID,
						UserMessageID:      run.UserMessageID,
						AssistantMessageID: run.AssistantMessageID,
						Model:              run.Model,
					}, status, streamResult, streamErrorText); err != nil {
						return runExecution{}, err
					}

					return runExecution{
						RunID:              run.RunID,
						AssistantMessageID: run.AssistantMessageID,
						Status:             status,
						ErrText:            streamErrorText,
					}, nil
				},
				func(execution runExecution, err error) {
					if activeRunID.Get() != run.RunID {
						return
					}
					activeRunID.Set("")
					activeAssistantID.Set("")
					isThinking.Set(false)

					if err != nil {
						errorText.Set(err.Error())
						messages.Set(setAssistantError(messages.Peek(), run.AssistantMessageID, err.Error()))
						return
					}

					messages.Set(markAssistantStatus(messages.Peek(), execution.AssistantMessageID, execution.Status))
					if execution.Status == "error" {
						errMessage := execution.ErrText
						if strings.TrimSpace(errMessage) == "" {
							errMessage = fmt.Sprintf("Model %s failed without a provider error message.", run.Model)
						}
						messages.Set(setAssistantError(messages.Peek(), execution.AssistantMessageID, errMessage))
					}
					if execution.ErrText != "" {
						errorText.Set(execution.ErrText)
					}
					loadChatsAction.Run(struct{}{})
				},
			)
		})

		onSend := func() {
			if activeRunID.Get() != "" {
				return
			}
			chatID := activeChatID.Get()
			if chatID == "" {
				return
			}
			content := strings.TrimSpace(inputText.Get())
			if content == "" {
				return
			}
			model := selectedModel.Get()
			if !chatService.IsAllowedModel(model) {
				model = chatService.DefaultModel()
				selectedModel.Set(model)
			}

			runID := uuid.NewString()
			userMessageID := uuid.NewString()
			assistantMessageID := uuid.NewString()
			now := time.Now().UTC()

			messages.Set(append(messages.Get(),
				MessageView{ID: userMessageID, Role: "user", Content: content, Status: "complete", CreatedAt: now},
				MessageView{ID: assistantMessageID, Role: "assistant", Content: "", Status: "streaming", CreatedAt: now},
			))
			inputText.Set("")
			isThinking.Set(true)
			errorText.Set("")
			activeRunID.Set(runID)
			activeAssistantID.Set(assistantMessageID)
			pendingRun.Set(PendingRun{
				RunID:              runID,
				ChatID:             chatID,
				UserMessageID:      userMessageID,
				AssistantMessageID: assistantMessageID,
				Model:              model,
				UserContent:        content,
			})
			runTrigger.Set(runTrigger.Get() + 1)
		}

		onStop := func() {
			runID := activeRunID.Get()
			assistantID := activeAssistantID.Get()
			if runID == "" || assistantID == "" {
				return
			}
			activeRunID.Set("")
			activeAssistantID.Set("")
			isThinking.Set(false)
			messages.Set(markAssistantStatus(messages.Get(), assistantID, "cancelled"))
		}

		onNewChat := func() {
			if activeRunID.Get() != "" {
				return
			}
			editingChatID.Set("")
			renameTitle.Set("")
			createChatAction.Run(selectedModel.Get())
		}

		onStartRename := func(chat chatsvc.Chat) {
			if activeRunID.Get() != "" {
				return
			}
			editingChatID.Set(chat.ID)
			renameTitle.Set(chat.Title)
			errorText.Set("")
		}

		onCancelRename := func() {
			editingChatID.Set("")
			renameTitle.Set("")
		}

		onSaveRename := func(chatID string) {
			if activeRunID.Get() != "" {
				return
			}
			renameChatAction.Run(renameChatRequest{
				ChatID: chatID,
				Title:  renameTitle.Get(),
			})
		}

		onDeleteChat := func(chatID string) {
			if activeRunID.Get() != "" {
				return
			}
			deleteChatAction.Run(chatID)
		}

		onToggleTheme := func() {
			if themeMode.Get() == "dark" {
				themeMode.Set("light")
				return
			}
			themeMode.Set("dark")
		}

		return func() *vango.VNode {
			chatList := chats.Get()
			messageList := messages.Get()
			activeChat := activeChatID.Get()
			running := activeRunID.Get() != ""
			thinking := isThinking.Get()
			selected := selectedModel.Get()
			errorMessage := errorText.Get()
			allowedModels := chatService.AllowedModels()
			palette := paletteFor(themeMode.Get())
			themeLabel := "Dark"
			if themeMode.Get() == "dark" {
				themeLabel = "Light"
			}

			var errorNode *vango.VNode
			if errorMessage != "" {
				errorNode = Div(Class("mb-2 text-sm "+palette.ErrorText), Text(errorMessage))
			}

			return Div(Class("h-screen chat-shell "+palette.AppRoot),
				Div(Class("h-full flex"),
					Aside(Class("w-80 flex flex-col "+palette.Sidebar),
						Div(Class("p-4 "+palette.SidebarSection),
							Button(
								Class("w-full rounded-md px-3 py-2 text-sm font-medium transition-colors "+palette.NewChatButton),
								OnClick(onNewChat),
								Disabled(running),
								Text("New Chat"),
							),
						),
						Div(Class("flex-1 overflow-y-auto p-2 space-y-2"),
							RangeKeyed(chatList,
								func(chat chatsvc.Chat) any { return chat.ID },
								func(chat chatsvc.Chat) *vango.VNode {
									buttonClass := palette.ChatButtonBase + " " + palette.ChatButtonIdle
									if chat.ID == activeChat {
										buttonClass = palette.ChatButtonBase + " " + palette.ChatButtonActive
									}
									isEditing := editingChatID.Get() == chat.ID
									if isEditing {
										return Div(Class(buttonClass+" space-y-2"),
											Input(
												Class("w-full rounded-md px-2 py-1 text-sm "+palette.ChatInput),
												Value(renameTitle.Get()),
												OnInput(func(value string) {
													renameTitle.Set(value)
												}),
											),
											Div(Class("flex gap-2"),
												Button(
													Class("rounded-md px-2 py-1 text-xs "+palette.ChatSaveButton),
													OnClick(func() {
														onSaveRename(chat.ID)
													}),
													Disabled(running || strings.TrimSpace(renameTitle.Get()) == ""),
													Text("Save"),
												),
												Button(
													Class("rounded-md px-2 py-1 text-xs "+palette.ChatActionButton),
													OnClick(onCancelRename),
													Disabled(running),
													Text("Cancel"),
												),
											),
										)
									}
									return Div(Class(buttonClass),
										Button(
											Class("w-full text-left"),
											OnClick(func() {
												activeChatID.Set(chat.ID)
												if chatService.IsAllowedModel(chat.Model) {
													selectedModel.Set(chat.Model)
												}
											}),
											Div(Class("truncate font-medium"), Text(chat.Title)),
											Div(Class("text-xs truncate mt-1 "+palette.ChatMeta), Text(chat.Model)),
										),
										Div(Class("mt-2 flex gap-2"),
											Button(
												Class("rounded-md px-2 py-1 text-xs "+palette.ChatActionButton),
												OnClick(func() {
													onStartRename(chat)
												}),
												Disabled(running),
												Text("Rename"),
											),
											Button(
												Class("rounded-md px-2 py-1 text-xs "+palette.ChatDangerButton),
												OnClick(func() {
													onDeleteChat(chat.ID)
												}),
												Disabled(running),
												Text("Delete"),
											),
										),
									)
								},
							),
						),
					),
					Div(Class("flex-1 flex flex-col min-w-0"),
						Div(Class("h-16 px-4 flex items-center justify-between gap-3 "+palette.Header),
							Div(Class("text-sm truncate "+palette.HeaderTitle), Text(fmt.Sprintf("Chat: %s", truncateText(activeChat, 8)))),
							Div(Class("flex items-center gap-2"),
								Select(
									Class("rounded-md px-2 py-1 text-sm "+palette.ModelSelect),
									Value(selected),
									OnInput(func(value string) {
										if chatService.IsAllowedModel(value) {
											selectedModel.Set(value)
										}
									}),
									RangeKeyed(allowedModels,
										func(model string) any { return model },
										func(model string) *vango.VNode {
											return Option(Value(model), Text(model))
										},
									),
								),
								Button(
									Class("rounded-md px-3 py-1.5 text-sm border transition-colors "+palette.ThemeToggle),
									OnClick(onToggleTheme),
									Text(themeLabel),
								),
								Button(
									Class("rounded-md px-3 py-1.5 text-sm border disabled:opacity-50 "+palette.StopButton),
									OnClick(onStop),
									Disabled(!running),
									Text("Stop"),
								),
							),
						),
						Div(Class("flex-1 overflow-y-auto p-4 space-y-4 "+palette.ChatBody),
							RangeKeyed(messageList,
								func(message MessageView) any { return message.ID },
								func(message MessageView) *vango.VNode {
									bubbleClass := "rounded-lg px-4 py-3 max-w-3xl whitespace-pre-wrap border"
									containerClass := "flex"
									if message.Role == "user" {
										containerClass += " justify-end"
										bubbleClass += " " + palette.UserBubble
									} else {
										containerClass += " justify-start"
										bubbleClass += " " + palette.AssistantBubble
									}

									statusBadge := ""
									if message.Status == "streaming" {
										statusBadge = "Streaming"
									}
									if message.Status == "error" {
										statusBadge = "Error"
									}
									if message.Status == "cancelled" {
										statusBadge = "Cancelled"
									}

									if message.Role == "assistant" && message.Content == "" && thinking {
										return Div(Class(containerClass),
											Div(Class(bubbleClass),
												Div(Class("text-sm "+palette.ThinkingText), Text("Thinking...")),
											),
										)
									}

									return Div(Class(containerClass),
										Div(Class(bubbleClass),
											Div(
												Class("text-[10px] mb-2 "+palette.StatusText),
												Attr("aria-hidden", "true"),
												If(statusBadge != "", Text(statusBadge)),
											),
											renderMessageContent(message, themeMode.Get(), palette),
											RangeKeyed(message.ToolCalls,
												func(call ToolCallView) any { return call.ID },
												func(call ToolCallView) *vango.VNode {
													var inputNode *vango.VNode
													var outputNode *vango.VNode
													var errNode *vango.VNode
													if call.Output != "" {
														outputNode = Div(Class(palette.ToolText), Text("Output: "+call.Output))
													}
													if call.ErrText != "" {
														errNode = Div(Class(palette.ToolErrorText), Text("Error: "+call.ErrText))
													}
													if call.Input != "" {
														inputNode = Div(Class(palette.ToolText), Text("Input: "+call.Input))
													}
													return Div(Class("mt-2 rounded-md border p-2 text-xs space-y-1 "+palette.ToolCard),
														Div(Class("font-semibold"), Text(fmt.Sprintf("Tool: %s (%s)", call.Name, call.Status))),
														inputNode,
														outputNode,
														errNode,
													)
												},
											),
										),
									)
								},
							),
						),
						Div(Class("p-4 "+palette.Composer),
							errorNode,
							Div(Class("flex items-end gap-2"),
								Textarea(
									Class("flex-1 min-h-24 max-h-60 rounded-md px-3 py-2 text-sm resize-y "+palette.Input),
									Placeholder("Ask anything..."),
									Value(inputText.Get()),
									OnInput(func(value string) {
										inputText.Set(value)
									}),
								),
								Button(
									Class("rounded-md px-4 py-2 text-sm font-semibold disabled:opacity-50 "+palette.SendButton),
									OnClick(onSend),
									Disabled(running || strings.TrimSpace(inputText.Get()) == ""),
									Text("Send"),
								),
							),
						),
					),
				),
			)
		}
	})
}

func containsChat(chats []chatsvc.Chat, chatID string) bool {
	for _, chat := range chats {
		if chat.ID == chatID {
			return true
		}
	}
	return false
}

func findChatByID(chats []chatsvc.Chat, chatID string) chatsvc.Chat {
	for _, chat := range chats {
		if chat.ID == chatID {
			return chat
		}
	}
	return chatsvc.Chat{}
}

func updateChatTitle(chats []chatsvc.Chat, chatID, title string) []chatsvc.Chat {
	next := make([]chatsvc.Chat, len(chats))
	copy(next, chats)
	for index := range next {
		if next[index].ID != chatID {
			continue
		}
		next[index].Title = title
		next[index].UpdatedAt = time.Now().UTC()
		break
	}
	return next
}

func removeChatByID(chats []chatsvc.Chat, chatID string) []chatsvc.Chat {
	next := make([]chatsvc.Chat, 0, len(chats))
	for _, chat := range chats {
		if chat.ID == chatID {
			continue
		}
		next = append(next, chat)
	}
	return next
}

func appendAssistantChunk(messages []MessageView, assistantMessageID, chunk string) []MessageView {
	next := make([]MessageView, len(messages))
	copy(next, messages)
	for index := range next {
		if next[index].ID != assistantMessageID {
			continue
		}
		next[index].Content += chunk
		next[index].Status = "streaming"
		break
	}
	return next
}

func markAssistantStatus(messages []MessageView, assistantMessageID, status string) []MessageView {
	next := make([]MessageView, len(messages))
	copy(next, messages)
	for index := range next {
		if next[index].ID != assistantMessageID {
			continue
		}
		next[index].Status = status
		break
	}
	return next
}

func setAssistantError(messages []MessageView, assistantMessageID, errMessage string) []MessageView {
	next := make([]MessageView, len(messages))
	copy(next, messages)
	for index := range next {
		if next[index].ID != assistantMessageID {
			continue
		}
		next[index].Status = "error"
		if strings.TrimSpace(errMessage) == "" {
			errMessage = "Assistant request failed."
		}
		if strings.TrimSpace(next[index].Content) == "" {
			next[index].Content = "Error: " + errMessage
		}
		break
	}
	return next
}

func addToolCall(messages []MessageView, assistantMessageID string, call ToolCallView) []MessageView {
	next := make([]MessageView, len(messages))
	copy(next, messages)
	for index := range next {
		if next[index].ID != assistantMessageID {
			continue
		}
		calls := append([]ToolCallView{}, next[index].ToolCalls...)
		calls = append(calls, call)
		next[index].ToolCalls = calls
		break
	}
	return next
}

func updateToolCall(messages []MessageView, assistantMessageID, callID, status, output, errorText string) []MessageView {
	next := make([]MessageView, len(messages))
	copy(next, messages)
	for messageIndex := range next {
		if next[messageIndex].ID != assistantMessageID {
			continue
		}
		calls := append([]ToolCallView{}, next[messageIndex].ToolCalls...)
		for callIndex := range calls {
			if calls[callIndex].ID != callID {
				continue
			}
			if status != "" {
				calls[callIndex].Status = status
			} else {
				calls[callIndex].Status = "completed"
			}
			calls[callIndex].Output = output
			calls[callIndex].ErrText = errorText
			next[messageIndex].ToolCalls = calls
			return next
		}
		if status == "" {
			status = "completed"
		}
		calls = append(calls, ToolCallView{ID: callID, Status: status, Output: output, ErrText: errorText})
		next[messageIndex].ToolCalls = calls
		return next
	}
	return next
}

func truncateText(value string, maxBytes int) string {
	if maxBytes <= 0 {
		return ""
	}
	if len(value) <= maxBytes {
		return value
	}
	if maxBytes <= 3 {
		return value[:maxBytes]
	}
	return value[:maxBytes-3] + "..."
}

func renderMessageContent(message MessageView, theme string, palette themePalette) *vango.VNode {
	if message.Role != "assistant" {
		return Div(Text(message.Content))
	}

	islandID := "md-" + message.ID
	return Div(
		Class("md-renderer-host"),
		Data("module", "/js/islands/markdown-renderer.js"),
		JSIsland(islandID, map[string]any{
			"markdown": message.Content,
			"theme":    theme,
		}),
		IslandPlaceholder(
			Div(Class("md-renderer "+palette.ToolText), Text(message.Content)),
		),
	)
}

func paletteFor(mode string) themePalette {
	if mode == "light" {
		return themePalette{
			AppRoot:          "bg-slate-100 text-slate-900",
			Sidebar:          "border-r border-slate-300 bg-slate-50",
			SidebarSection:   "border-b border-slate-300",
			NewChatButton:    "bg-slate-800 text-white hover:bg-slate-700",
			ChatButtonBase:   "w-full text-left rounded-md px-3 py-2 text-sm transition-colors border",
			ChatButtonIdle:   "bg-white border-slate-300 hover:bg-slate-100",
			ChatButtonActive: "bg-blue-100 border-blue-400",
			ChatActionButton: "border border-slate-300 bg-white text-slate-700 hover:bg-slate-100",
			ChatDangerButton: "border border-red-300 bg-white text-red-700 hover:bg-red-100",
			ChatInput:        "bg-white border border-slate-300 text-slate-900",
			ChatSaveButton:   "border border-blue-300 bg-blue-600 text-white hover:bg-blue-700",
			ChatMeta:         "text-slate-500",
			Header:           "border-b border-slate-300 bg-white",
			HeaderTitle:      "text-slate-700",
			ModelSelect:      "bg-white border border-slate-300 text-slate-900",
			ThemeToggle:      "border-slate-300 text-slate-700 hover:bg-slate-100",
			StopButton:       "border-red-300 text-red-700 hover:bg-red-100",
			ErrorText:        "text-red-700",
			ChatBody:         "bg-white",
			AssistantBubble:  "bg-transparent border-transparent text-slate-900",
			UserBubble:       "bg-slate-200 border-[#2445FF] text-slate-900",
			ThinkingText:     "text-slate-600",
			StatusText:       "text-slate-500",
			RoleText:         "text-slate-600",
			ToolCard:         "border-slate-300 bg-slate-100",
			ToolText:         "text-slate-700",
			ToolErrorText:    "text-red-700",
			Composer:         "border-t border-slate-300 bg-white",
			Input:            "bg-white border border-slate-300 text-slate-900 placeholder:text-slate-500",
			SendButton:       "bg-blue-600 text-white hover:bg-blue-700",
		}
	}

	return themePalette{
		AppRoot:          "bg-[#0b1320] text-white",
		Sidebar:          "border-r border-white/10 bg-black",
		SidebarSection:   "border-b border-white/10",
		NewChatButton:    "bg-zinc-900 hover:bg-zinc-800 text-white",
		ChatButtonBase:   "w-full text-left rounded-md px-3 py-2 text-sm transition-colors border border-transparent",
		ChatButtonIdle:   "bg-zinc-950 hover:bg-zinc-900",
		ChatButtonActive: "bg-zinc-900 border-white/20",
		ChatActionButton: "border border-white/20 bg-zinc-950 text-white/90 hover:bg-zinc-900",
		ChatDangerButton: "border border-red-500/40 bg-zinc-950 text-red-200 hover:bg-red-500/10",
		ChatInput:        "bg-zinc-950 border border-white/20 text-white",
		ChatSaveButton:   "border border-blue-400/50 bg-[#2457d6] text-white hover:bg-[#2e63e0]",
		ChatMeta:         "text-white/60",
		Header:           "border-b border-white/10 bg-black",
		HeaderTitle:      "text-white/80",
		ModelSelect:      "bg-zinc-950 border border-white/20 text-white",
		ThemeToggle:      "border-white/30 text-white hover:bg-white/10",
		StopButton:       "border-red-400/40 text-red-200 hover:bg-red-400/10",
		ErrorText:        "text-red-300",
		ChatBody:         "bg-black",
		AssistantBubble:  "bg-transparent border-transparent text-white",
		UserBubble:       "bg-zinc-900 border-[#2445FF] text-white",
		ThinkingText:     "text-white/70",
		StatusText:       "text-white/50",
		RoleText:         "text-white/60",
		ToolCard:         "border-white/10 bg-black/20",
		ToolText:         "text-white/70",
		ToolErrorText:    "text-red-200",
		Composer:         "border-t border-white/10 bg-black",
		Input:            "bg-zinc-950 border border-white/20 text-white placeholder:text-white/60",
		SendButton:       "bg-[#2457d6] text-white hover:bg-[#2e63e0]",
	}
}
