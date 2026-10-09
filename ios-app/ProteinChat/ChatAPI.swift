import Foundation

struct ChatMessage: Codable, Identifiable {
    var id = UUID()
    var role: String
    var content: String
    enum CodingKeys: String, CodingKey { case role, content }
}
struct ChatModel: Decodable, Identifiable { let id: String; let name: String? }
struct Conversation: Decodable, Identifiable { let id: String; let title: String; let model: String }
struct ModelList: Decodable { let models: [ChatModel]; let `default`: String }
struct ConversationList: Decodable { let conversations: [Conversation] }
struct MessageList: Decodable { let messages: [ChatMessage] }
struct ChatBody: Encodable { let conversation_id: String; let model: String; let messages: [ChatMessage] }
struct ChatFailure: LocalizedError { let message: String; var errorDescription: String? { message } }

// URLSession.AsyncBytes.lines handles UTF-8 across transport chunk boundaries.
struct SSEParser {
    private(set) var done = false
    mutating func consume(_ line: String) throws -> String? {
        guard !done, line.hasPrefix("data:") else { return nil }
        let payload = String(line.dropFirst(5)).trimmingCharacters(in: .whitespacesAndNewlines)
        if payload == "[DONE]" { done = true; return nil }
        let object = try JSONSerialization.jsonObject(with: Data(payload.utf8))
        guard let event = object as? [String: Any] else { throw ChatFailure(message: "不支持的消息格式") }
        if let error = event["error"] { throw ChatFailure(message: error as? String ?? String(describing: error)) }
        return event["delta"] as? String ?? event["content"] as? String
    }
    func finish() throws {
        if !done { throw ChatFailure(message: "连接提前结束，回复可能不完整") }
    }
}
struct ChatAPI {
    let base: String
    let key: String
    static func normalize(_ value: String) throws -> String {
        let trimmed = value.trimmingCharacters(in: .whitespacesAndNewlines)
        guard var parts = URLComponents(string: trimmed), parts.scheme == "https", parts.host?.isEmpty == false,
              parts.user == nil, parts.password == nil, parts.query == nil, parts.fragment == nil,
              parts.path.isEmpty || parts.path == "/" else {
            throw ChatFailure(message: "请输入 HTTPS 站点地址，例如 https://linpro.top（不要加 /api）")
        }
        parts.path = ""
        return parts.string!
    }
    func request(_ path: String) throws -> URLRequest {
        guard let url = URL(string: base + "/api" + path) else { throw ChatFailure(message: "服务地址无效") }
        var request = URLRequest(url: url)
        request.timeoutInterval = 120
        if !key.isEmpty { request.setValue(key, forHTTPHeaderField: "X-API-Key") }
        request.setValue("application/json", forHTTPHeaderField: "Content-Type")
        return request
    }
    func get<T: Decodable>(_ path: String) async throws -> T {
        let (data, response) = try await URLSession.shared.data(for: request(path))
        try check(response, detail: String(data: data, encoding: .utf8) ?? "")
        return try JSONDecoder().decode(T.self, from: data)
    }
    func check(_ response: URLResponse, detail: String = "") throws {
        guard let http = response as? HTTPURLResponse else { throw ChatFailure(message: "无效的服务器响应") }
        guard (200..<300).contains(http.statusCode) else {
            throw ChatFailure(message: "HTTP \(http.statusCode)：\(detail.isEmpty ? "请检查 API Key、模型权限及服务状态" : String(detail.prefix(400)))")
        }
    }
    func stream(_ body: ChatBody, onDelta: @MainActor (String) -> Void) async throws {
        var request = try request("/chat/stream")
        request.httpMethod = "POST"
        request.setValue("text/event-stream", forHTTPHeaderField: "Accept")
        request.httpBody = try JSONEncoder().encode(body)
        let (bytes, response) = try await URLSession.shared.bytes(for: request)
        try check(response)
        var parser = SSEParser()
        for try await line in bytes.lines {
            try Task.checkCancellation()
            if let delta = try parser.consume(line) { await onDelta(delta) }
            if parser.done { break }
        }
        try Task.checkCancellation()
        try parser.finish()
    }
}
