import Foundation

func check(_ condition: @autoclosure () -> Bool) { precondition(condition()) }
var parser = SSEParser()
let delta = try parser.consume("data: {\"delta\":\"你好🌏\"}")
check(delta == "你好🌏")
_ = try parser.consume(": heartbeat")
_ = try parser.consume("data: [DONE]")
try parser.finish()
check(parser.done)
let ignored = try parser.consume("data: invalid")
check(ignored == nil)
do { var p = SSEParser(); _ = try p.consume("data: {\"error\":\"denied\"}"); fatalError("must throw") }
catch { check(error.localizedDescription == "denied") }
do { try SSEParser().finish(); fatalError("must throw") } catch {}
let base = try ChatAPI.normalize(" https://linpro.top/ ")
check(base == "https://linpro.top")
for value in ["http://linpro.top", "https://user@linpro.top", "https://linpro.top/api", "https://linpro.top?q=1"] {
    do { _ = try ChatAPI.normalize(value); fatalError("invalid origin accepted") } catch {}
}
let api = ChatAPI(base: base, key: "test-key")
let request = try api.request("/models")
check(request.url?.absoluteString == "https://linpro.top/api/models")
check(request.value(forHTTPHeaderField: "X-API-Key") == "test-key")
let messages = try JSONDecoder().decode(MessageList.self, from: Data("{\"messages\":[{\"role\":\"user\",\"content\":\"你好\"}]}".utf8))
check(messages.messages[0].content == "你好")
let body = ChatBody(conversation_id: "test", model: "model", messages: messages.messages)
let data = try JSONEncoder().encode(body)
let object = try JSONSerialization.jsonObject(with: data) as! [String: Any]
check((object["messages"] as! [[String: Any]])[0]["id"] == nil)
print("Swift API / SSE checks passed")
