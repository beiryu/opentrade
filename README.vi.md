# OpenTrade

**Terminal giao dịch mã nguồn mở cho chứng khoán Việt Nam, nơi chiến lược là một mô hình ngôn ngữ, và nơi câu nói đó được đo chứ không được tin.**

[English](README.md) · [Kiến trúc](docs/architecture.md) · [Ghi chú khảo sát](docs/research/) · [Lộ trình](docs/research/roadmap.md)

OpenTrade ra một quyết định mỗi phiên. Sau giờ đóng cửa, nó gom tài khoản, thị trường và danh sách cổ phiếu ứng viên thành một prompt, hỏi model danh sách lệnh dạng JSON, lọc qua các chốt an toàn viết bằng code, rồi đưa cho một **broker giả lập** áp dụng đúng luật HOSE/HNX: lô 100, bước giá, biên độ ngày, T+2, phí và thuế. Mỗi lần gọi được lưu nguyên prompt, câu trả lời thô của model và những gì guard đã sửa.

Rồi nó làm điều mà các dự án LLM trading nó học theo không làm: `opentrade replay` chạy vòng đó ngày qua ngày trên dữ liệu lịch sử và chấm điểm so với **mua và giữ VN-Index** và **một bot đánh ngẫu nhiên** dưới cùng ràng buộc.

```
SCORECARD 2026-01-02 → 2026-10-09
               strategy  VN-Index  random
total return   +12.53%   +17.13%   -1.18%
max drawdown   3.54%     6.87%     6.84%
sharpe         1.61      1.51      -0.19
win rate       49%       -         35%

Beat random trading but not the index: holding VN-Index would have done better.
```

Đó là baseline momentum trên dữ liệu mẫu **tổng hợp**. Nó ở đây để cho thấy đầu ra, không để chứng minh gì.

## Là gì, không là gì

- **Là** một dụng cụ đo cho câu hỏi "model có đánh được chứng khoán VN không?", có sẵn luật thị trường, log minh bạch từng quyết định, và hai baseline mà chiến lược nào cũng phải vượt.
- **Là** paper-first. Hôm nay không có gì nối với broker thật. Adapter đặt lệnh thật nằm trong lộ trình và sẽ luôn phải bật tay, sau cùng một lớp guard.
- **Không là** bot kiếm tiền. Chưa ai chứng minh LLM đọc chỉ báo có lợi thế, và dữ liệu mẫu là bịa. Mọi kết quả là thí nghiệm.
- **Không là** fork. Thiết kế theo [NOFX](https://github.com/NoFxAiOS/nofx) (cycle → prompt → JSON → guards → execute, log mọi thứ); code viết mới và MIT, vì 90 % NOFX là máy móc crypto perpetual mà thị trường VN không dùng.

## Bắt đầu nhanh

Cần Go 1.26+ (Go cũ hơn tự tải) và, cho web app, Node 20 với pnpm.

```bash
git clone https://github.com/beiryu/opentrade && cd opentrade
go run ./cmd/opentrade seed                 # dữ liệu mẫu tổng hợp vào data/sample
make replay                                 # baseline momentum cho 2026, in scorecard
make build && ./opentrade serve             # http://127.0.0.1:8080 kèm web app
```

Để model ra quyết định, trỏ tới một endpoint tương thích OpenAI:

```bash
cp .env.example .env                        # điền OPENTRADE_LLM_API_KEY
./opentrade replay --from 2026-07-01 --to 2026-09-30 --decider llm
./opentrade cycle --decider llm             # một phiên trên trạng thái đã lưu
```

Mặc định là [Vercel AI Gateway](https://vercel.com/docs/ai-gateway) với `deepseek/deepseek-v4.1-flash`. Đổi `OPENTRADE_LLM_BASE_URL` là chạy được DeepSeek trực tiếp, OpenAI, Ollama, LM Studio.

## Một phiên diễn ra thế nào

1. **Context**: tài khoản (tiền, tiền chờ về, vị thế với số đã về), VN-Index, ứng viên với giá đóng, sàn/trần phiên sau, SMA20/50, RSI14, ATR14, return 5/20 phiên, tỉ lệ khối lượng, 10 lệnh gần nhất.
2. **Decider**: `llm` | `momentum` | `random` → JSON `{"decisions":[{symbol, action, lots, limit_price, confidence, reasoning}]}`.
3. **Guards**: mã trong universe · một lệnh mỗi mã · lots > 0 · giá đúng bước và trong biên độ · bán ≤ số đã về (T+2) · mua ≤ tiền và ≤ 30 % equity mỗi mã · max vị thế · max lệnh · safe mode sau 3 lần decider lỗi (chỉ cho bán).
4. **Broker giả lập**: lệnh ngày cho D+1, khớp ở giá mở hoặc giá giới hạn, phí 0,15 %, thuế bán 0,1 %, cổ phiếu và tiền về T+2.
5. **Ghi**: prompt, trả lời thô, lệnh sau guard, guard log, kết quả lệnh, equity cuối phiên.

Chi tiết và bản đồ package trong [docs/architecture.md](docs/architecture.md).

## Dữ liệu

`data/sample` là tổng hợp (xem [README của nó](data/sample/README.md)). Bất kỳ thư mục `MÃ.csv` với cột `date,open,high,low,close,volume` đều chạy được qua `OPENTRADE_DATA_DIR`. Provider cho DNSE LightSpeed và SSI FastConnect là hai mục đầu của [lộ trình](docs/research/roadmap.md).

## Bảo mật và miễn trừ

Chạy trên máy bạn: bind `127.0.0.1`, không có đăng nhập, bí mật duy nhất là API key model nằm trong môi trường, không bao giờ trong repo. Báo lỗ hổng qua [GitHub security advisories](https://github.com/beiryu/opentrade/security/advisories/new).

Phần mềm dành cho nghiên cứu và học tập. Giao dịch tự động có thể mất tiền; model có thể tự tin mà sai. Không phải lời khuyên đầu tư. Dữ liệu mẫu là hư cấu.

## License

[MIT](LICENSE) © 2026 beiryu.
