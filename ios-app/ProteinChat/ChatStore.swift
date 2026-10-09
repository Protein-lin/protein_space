import Foundation
import Combine

@MainActor final class ChatStore: ObservableObject {
    @Published var base = UserDefaults.standard.string(forKey: "server") ?? "https://linpro.top"
    @Published var key = Keychain.read()
    @Published var models: [ChatModel] = []
    @Published var model = ""
    @Published var messages: [ChatMessage] = []
    @Published var conversations: [Conversation] = []
    @Published var input = ""
    @Published var error = ""
    @Published var connected = false
    @Published var busy = false
    @Published var loading = false
    @Published var settings = true
    @Published var history = false
    private var conversationID = UUID().uuidString
    private var api: ChatAPI?
    private var generation: Task<Void, Never>?

    func connect() async {
        guard !busy && !loading else { return }
        loading = true; error = ""; connected = false; models = []; conversations = []
        defer { loading = false }
        do {
            let normalized = try ChatAPI.normalize(base)
            let client = ChatAPI(base: normalized, key: key.trimmingCharacters(in: .whitespacesAndNewlines))
            let result: ModelList = try await client.get("/models")
            guard !result.models.isEmpty else { throw ChatFailure(message: "没有可用模型，请检查账号模型权限") }
            try Keychain.save(client.key)
            UserDefaults.standard.set(normalized, forKey: "server")
            base = normalized; key = client.key; api = client; models = result.models
            model = models.contains(where: { $0.id == result.default }) ? result.default : models[0].id
            connected = true; settings = false; newChat()
        } catch { self.error = error.localizedDescription }
    }
    func newChat() {
        guard !busy else { return }
        conversationID = UUID().uuidString; messages = []; input = ""; error = ""; history = false
    }
    func loadHistory() async {
        guard let api, connected, !busy, !loading else { return }
        loading = true; error = ""
        defer { loading = false }
        do {
            let result: ConversationList = try await api.get("/conversations")
            conversations = result.conversations; history = true
        } catch { self.error = error.localizedDescription }
    }
    func open(_ conversation: Conversation) async {
        guard let api, !loading, !busy else { return }
        loading = true; error = ""
        defer { loading = false }
        do {
            var parts = URLComponents(); parts.queryItems = [URLQueryItem(name: "id", value: conversation.id)]
            let result: MessageList = try await api.get("/conversations/messages?" + (parts.percentEncodedQuery ?? ""))
            conversationID = conversation.id; messages = result.messages
            if models.contains(where: { $0.id == conversation.model }) { model = conversation.model }
            input = ""; history = false
        } catch { self.error = error.localizedDescription; history = false }
    }
    func send() {
        let text = input.trimmingCharacters(in: .whitespacesAndNewlines)
        guard let api, connected, !busy, !loading, !text.isEmpty, !model.isEmpty else { return }
        messages.removeAll { $0.content.isEmpty }
        messages.append(ChatMessage(role: "user", content: text))
        let body = ChatBody(conversation_id: conversationID, model: model, messages: messages)
        let assistant = ChatMessage(role: "assistant", content: "")
        messages.append(assistant); input = ""; error = ""; busy = true
        generation = Task {
            defer { busy = false; generation = nil }
            do {
                try await api.stream(body) { [weak self] delta in
                    guard let self, let index = self.messages.firstIndex(where: { $0.id == assistant.id }) else { return }
                    self.messages[index].content += delta
                }
            } catch {
                self.error = Task.isCancelled ? "已停止生成，回复可能不完整" : error.localizedDescription
            }
        }
    }
    func stop() { generation?.cancel() }
}
