import SwiftUI

struct ChatView: View {
    @StateObject private var store = ChatStore()
    var body: some View {
        NavigationStack {
            VStack(spacing: 0) {
                HStack {
                    if store.models.isEmpty { Text("请先连接服务").foregroundStyle(.secondary) }
                    else {
                        Picker("模型", selection: $store.model) {
                            ForEach(store.models) { Text($0.name ?? $0.id).tag($0.id) }
                        }.labelsHidden().disabled(store.busy || store.loading)
                    }
                    Spacer()
                    Button { Task { await store.loadHistory() } } label: { Image(systemName: "clock.arrow.circlepath") }
                        .accessibilityLabel("历史记录").disabled(!store.connected || store.busy || store.loading)
                    Button { store.newChat() } label: { Image(systemName: "square.and.pencil") }
                        .accessibilityLabel("新对话").disabled(store.busy || store.loading)
                }.padding(.horizontal).padding(.vertical, 10)
                if !store.error.isEmpty {
                    Text(store.error).font(.footnote).foregroundStyle(.red).textSelection(.enabled)
                        .padding(12).frame(maxWidth: .infinity, alignment: .leading).background(Color.red.opacity(0.07))
                }
                if store.loading { ProgressView().padding(8) }
                ScrollViewReader { proxy in
                    ScrollView {
                        LazyVStack(alignment: .leading, spacing: 20) {
                            if store.messages.isEmpty { welcome }
                            ForEach(store.messages) { message in
                                VStack(alignment: message.role == "user" ? .trailing : .leading, spacing: 6) {
                                    Text(message.role == "user" ? "你" : "Protein").font(.caption).foregroundStyle(.secondary)
                                    Text(message.content.isEmpty ? (store.busy ? "正在思考…" : "未收到回复") : message.content)
                                        .textSelection(.enabled).padding(16)
                                        .background(message.role == "user" ? Color.blue.opacity(0.12) : Color(uiColor: .secondarySystemGroupedBackground))
                                        .clipShape(RoundedRectangle(cornerRadius: 18))
                                }.frame(maxWidth: .infinity, alignment: message.role == "user" ? .trailing : .leading)
                            }
                            Color.clear.frame(height: 1).id("bottom")
                        }.padding()
                    }.onChange(of: store.messages.last?.content) { _ in proxy.scrollTo("bottom", anchor: .bottom) }
                        .onChange(of: store.messages.count) { _ in proxy.scrollTo("bottom", anchor: .bottom) }
                }
            }
            .background(Color(uiColor: .systemGroupedBackground))
            .navigationTitle("Protein Space").navigationBarTitleDisplayMode(.inline)
            .toolbar { ToolbarItem(placement: .navigationBarTrailing) {
                Button { store.settings = true } label: { Image(systemName: "gearshape") }
                    .accessibilityLabel("设置").disabled(store.busy || store.loading)
            } }
            .safeAreaInset(edge: .bottom) { composer }
            .sheet(isPresented: $store.settings) { settings }
            .sheet(isPresented: $store.history) { history }
        }.tint(.blue)
    }
    private var welcome: some View {
        VStack(spacing: 16) {
            Image(systemName: "bubble.left.and.bubble.right.fill").font(.system(size: 48)).foregroundStyle(.blue)
            Text("今天想聊些什么？").font(.title2.bold())
            Text("提一个问题，让想法更进一步。").foregroundStyle(.secondary)
        }.frame(maxWidth: .infinity).padding(.vertical, 80)
    }
    private var composer: some View {
        HStack(alignment: .bottom, spacing: 12) {
            TextField("输入消息…", text: $store.input, axis: .vertical)
                .lineLimit(1...6).padding(12).background(.background).clipShape(RoundedRectangle(cornerRadius: 16))
                .disabled(store.busy)
            if store.busy {
                Button("停止", action: store.stop).buttonStyle(.bordered)
            } else {
                Button(action: store.send) { Image(systemName: "arrow.up").font(.headline).padding(6) }
                    .buttonStyle(.borderedProminent).accessibilityLabel("发送")
                    .disabled(!store.connected || store.loading || store.input.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty)
            }
        }.padding().background(.ultraThinMaterial)
    }
    private var settings: some View {
        NavigationStack {
            Form {
                Section("服务连接") {
                    TextField("https://linpro.top", text: $store.base).keyboardType(.URL).textInputAutocapitalization(.never).autocorrectionDisabled()
                    SecureField("个人 API Key（waf_…）", text: $store.key).textInputAutocapitalization(.never).autocorrectionDisabled()
                }
                Section {
                    Text("在网页账户中创建个人 API Key。Key 保存于本机钥匙串；IP 白名单用户可留空。服务地址不要添加 /api。").font(.footnote).foregroundStyle(.secondary)
                    if !store.error.isEmpty { Text(store.error).foregroundStyle(.red).font(.footnote) }
                    Button { Task { await store.connect() } } label: {
                        HStack { Text("连接服务"); if store.loading { Spacer(); ProgressView() } }
                    }
                }
            }.disabled(store.loading).navigationTitle("连接设置")
                .toolbar { ToolbarItem(placement: .confirmationAction) { Button("完成") { store.settings = false }.disabled(store.loading) } }
        }.interactiveDismissDisabled(store.loading)
    }
    private var history: some View {
        NavigationStack {
            List {
                if store.conversations.isEmpty { Text("暂无历史对话").foregroundStyle(.secondary) }
                ForEach(store.conversations) { conversation in
                    Button { Task { await store.open(conversation) } } label: {
                        VStack(alignment: .leading, spacing: 6) {
                            Text(conversation.title.isEmpty ? "新对话" : conversation.title).foregroundStyle(.primary)
                            Text(conversation.model).font(.caption).foregroundStyle(.secondary)
                        }
                    }.disabled(store.loading)
                }
            }.overlay { if store.loading { ProgressView() } }.navigationTitle("历史记录")
                .toolbar { ToolbarItem(placement: .confirmationAction) { Button("关闭") { store.history = false } } }
        }
    }
}
