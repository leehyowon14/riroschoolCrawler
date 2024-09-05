package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
)

type assignmentInfo struct {
	Number string `json:"number"` //과제의 타입 (1: 수행평가, 2:경시대회, 3: 포트폴리오) - 각 과제마다 주어진 순번
	Teacher string `json:"teacher"` //선생님 성함 (여러명일경우 전부 다)
	Subject string `json:"subject"` //과목
	Title string `json:"title"` //수행평가 이름
	Time int64 `json:"time"` //제출기한(unix)
	IsEnded bool `json:"isEnded"` //마감여부
	// isDone bool //제출여부
}
type result struct {
	Success bool `json:"success"`
	LastUpdate int64 `json:"lastUpdate"` // 마지막 업데이트 시간 (현재 UNIX 시간)
	FirstGrade []assignmentInfo `json:"firstGrade"`
	SecondGrade []assignmentInfo `json:"secondGrade"`
	ThirdGrade []assignmentInfo `json:"thirdGrade"`
}

type loginResult struct {
	Code int
}
type loginInfo struct {
	// 외부 패키지에서 접근 가능하여면 대문자로 시작해야함.
	Name string
	Link string
	Id string
	Password string
}
type jsonData []loginInfo

var (
	errUnmatch = errors.New("id/pw unmatched")
	errLocked = errors.New("your account is locked")
	errIdNotExists = errors.New("your id doesn't exist")
	errConnectionFailed = errors.New("unable to connect to login API")
	errAuthTokenNotFounded = errors.New("unable to load Auth token")
	errCantGetLastPage = errors.New("unable to get last page")
)

func main() {
	fmt.Println("Loading Data.json...") 
	data := loadJSON() 
	fmt.Println("Done!")
	resultChannel := make(chan *result)
	for _, loginData := range data { 
		go getSchoolAssignment(loginData, resultChannel) 
		fmt.Println(loginData.Name+": Crawling Started")
	} //계정별 루프
	
	for _, loginData := range data {
		saveJSON(loginData.Name, <-resultChannel)
	}
}

// loadJSON : Data.json을 불러옵니다.
func loadJSON() (loadedData jsonData) {
	jsonContent, err := os.ReadFile("Data.json")
	if err != nil {
		log.Fatalln("Error occurred while load Data.json: ", err)
	}
	json.Unmarshal(jsonContent, &loadedData)
	return
}

//login : loginInfo의 Link, Id, Password를 인자로 받아 인증에 필요한 쿠키 문자열을 반환합니다.
func login(link, id, pw string) (map[string]string, error) {
	body := []byte("app=user&mode=login&userType=1&id="+url.QueryEscape(id)+"&pw="+url.QueryEscape(pw)+"&deeplink=&redirect_link=")
		
	client := &http.Client{}
	req, err := http.NewRequest("POST", "https://"+link+"/ajax.php", bytes.NewBuffer(body))
	if err != nil {
		log.Fatalln("Error occurred while make new request: ", err)
	}
	req.Header.Add("Host", link)
	req.Header.Add("Content-Length", string(len(body)))
	req.Header.Add("content-type", "application/x-www-form-urlencoded; charset=UTF-8")

	res, err := client.Do(req)
	if err != nil {
		log.Fatalln("Error occurred while connecting login server: ", err)
	}
	res.Body.Close()

	cookies := make(map[string]string)
	if res.StatusCode != 200 {
		return cookies, errConnectionFailed
	}

	bodyBytes, _ := io.ReadAll(res.Body)
	var loginResult loginResult
	json.Unmarshal(bodyBytes, &loginResult)
	switch loginResult.Code {
	case 902:
		return cookies, errUnmatch
	case 777:
		return cookies, errLocked
	case 400:
		return cookies, errIdNotExists
	case 0o0:
		for _, cookie := range res.Cookies() {
			switch cookie.Name {
			case "cookie_token":
				cookies["cookie_token"] = cookie.Value
			case "login_chk":
				cookies["login_chk"] = cookie.Value
			case "device_id":
				cookies["device_id"] = cookie.Value
			}
		}
		if len(cookies) != 3 {
			return make(map[string]string), errAuthTokenNotFounded
		}

		return cookies, nil
	
	default:
		return cookies, errConnectionFailed
	}
}

func getAdjustedYear() int {
	currentYear := time.Now().Year()
	if time.Now().Month() < 3 {
		return currentYear - 1 
	}
	return currentYear
}

func getSchoolAssignment(loginData loginInfo, resultChannel chan<- *result) {
	resultData := &result{}
	resultData.Success = false
	cookies, err := login(loginData.Link, loginData.Id, loginData.Password)
	if err != nil {
		fmt.Println(loginData.Name+": ", err)
		resultChannel <- resultData
	}

	gradeChannels := make([]chan []assignmentInfo, 3)
	for i := range gradeChannels {
		gradeChannels[i] = make(chan []assignmentInfo)
		go getGradeAssignment(loginData, cookies, i+1, gradeChannels[i])
	}
	for i := range gradeChannels {
		var resultArray *[]assignmentInfo
		
		switch i + 1 {
		case 1:
			resultArray = &resultData.FirstGrade
		case 2:
			resultArray = &resultData.SecondGrade
		case 3:
			resultArray = &resultData.ThirdGrade
		}
		*resultArray = <-gradeChannels[i]
	}
	resultData.Success = true
	resultData.LastUpdate = time.Now().Unix()

	resultChannel <- resultData
}

func getGradeAssignment(loginData loginInfo, cookies map[string]string, grade int, gradeAssignmentChannel chan<- []assignmentInfo) {
	assignmentCategoryCode := [3]string{
		"1551", //수행평가
		"1552", //경시대회
		"1502", //포트폴리오
	}

	categoryAssignmentChannel := make(chan []assignmentInfo)
	for categoryIdx, categoryCode := range assignmentCategoryCode {
		go getCategoryAssignment(loginData, cookies, categoryIdx, categoryCode, grade, categoryAssignmentChannel)
	} //과제방별 루프
	
	pageCrawlResult := <-categoryAssignmentChannel
	for categoryIdx := 0; categoryIdx < len(assignmentCategoryCode) - 1; categoryIdx++ {
		pageAssignments := <-categoryAssignmentChannel
		pageCrawlResult = append(pageCrawlResult, pageAssignments...)
	}
	gradeAssignmentChannel <- pageCrawlResult
}

func getCategoryAssignment(loginData loginInfo, cookies map[string]string, categoryIdx int, categoryCode string, grade int, categoryAssignmentChannel chan<- []assignmentInfo) {

	lastPage, err := checkLastPage(cookies, loginData, categoryCode, grade)
	if err != nil {
		fmt.Println(loginData.Name+": can't get last page", err)
	}

	pageAssignmetChannel := make(chan []assignmentInfo)
	for pageNum := 1; pageNum <= lastPage; pageNum++ {
		go getPageAssignment(loginData, cookies, categoryIdx, categoryCode, grade, pageNum, pageAssignmetChannel)
	} 

	pageAssignmentsArray := make([]assignmentInfo, 0)
	for pageNum := 1; pageNum <= lastPage; pageNum++ {
		pageAssignments := <-pageAssignmetChannel
		if pageAssignments != nil {
			pageAssignmentsArray = append(pageAssignmentsArray, pageAssignments...)
		}
	} 
	categoryAssignmentChannel <- pageAssignmentsArray
}

func getPageAssignment(loginData loginInfo, cookies map[string]string, categoryIdx int, categoryCode string, grade int, pageNum int, pageAssignmetChannel chan []assignmentInfo) {
	//https://학교이름.riroschool.kr/portfolio.php?club=index&action=categoryIdx&db=1551&t_grade=학년&page=페이지&
	client := &http.Client{}
	
	params := url.Values{}
	params.Add("db", categoryCode)
	params.Add("t_grade", strconv.Itoa(grade))
	params.Add("page", strconv.Itoa(pageNum)) 
	params.Add("t_year", strconv.Itoa(getAdjustedYear()))
	encodedParams := params.Encode()
	req, err := http.NewRequest("GET", fmt.Sprintf("https://%s/portfolio.php?%s", loginData.Link, encodedParams), nil)
	if err != nil {
		fmt.Println(loginData.Name+": "+string(categoryIdx)+"-p."+string(pageNum), err)
		pageAssignmetChannel <- nil
	}
	for name, value := range cookies{
		req.AddCookie(&http.Cookie{Name: name, Value: value})
	}

	res, err := client.Do(req)
	if err != nil {
		fmt.Println(loginData.Name+": "+string(categoryIdx)+"-p."+string(pageNum), err)
		pageAssignmetChannel <- nil
	}

	doc, err := goquery.NewDocumentFromReader(res.Body)
	if err != nil {
		fmt.Println(loginData.Name+": "+string(categoryIdx)+"-p."+string(pageNum), err)
		pageAssignmetChannel <- nil
	}
	
	pageResultArray := make([]assignmentInfo,0)
	doc.Find("#container > div > div.renewal_wrap.portfolio_wrap > table > tbody > tr").Each(func(i int, s *goquery.Selection) {
		if i == 0 {
			return
		}

		assignment := &assignmentInfo{}
		
		//순번
		assignment.Number = strconv.Itoa(categoryIdx + 1) + "-" + s.Find("td:nth-child(1)").Text()

		//선생님 성함
		assignment.Teacher = s.Find("td:nth-child(6)").Text()
		
		//과목 & 과제명
		fullTitleSlice := strings.Split(s.Find(".txt").Text(), "  - ")
		assignment.Subject = strings.Fields(fullTitleSlice[0])[2]
		assignment.Title = fullTitleSlice[1]
		
		//제출기한
		dateString := s.Find("td:nth-child(7) > strong").Text()
		registerMonth, _ := strconv.Atoi(strings.Split(s.Find("td:nth-child(7)").First().Text(), "-")[0])
		loc, _ := time.LoadLocation("Asia/Seoul")
		month, _ := strconv.Atoi(strings.Split(dateString, "-")[0])
		year := time.Now().Local().Year()
		if time.Month(registerMonth) > time.Month(month) { //last month > this assign month //마감일 순서 아님.
			year += 1
		}
		date, _ := time.ParseInLocation(time.DateTime, fmt.Sprintf("%d-%s", year, dateString), loc)
		assignment.Time = date.Unix()

		//마감 여부
		if s.Find(".state > p > span").Text() == "마감" {
			assignment.IsEnded = true
		} else {
			assignment.IsEnded = false
		}
		
		//제출 여부
		//assignment.isDone = false

		pageResultArray = append(pageResultArray, *assignment) //resultArray 앞에 별
	})
	pageAssignmetChannel <- pageResultArray
	res.Body.Close()
}

func checkLastPage(cookies map[string]string, loginData loginInfo, categoryCode string, grade int) (int, error) {
	client := &http.Client{}

	params := url.Values{}
	params.Add("db", categoryCode)
	params.Add("t_grade", strconv.Itoa(grade))
	params.Add("page", "1") 
	params.Add("t_year", strconv.Itoa(getAdjustedYear()))
	encodedParams := params.Encode()
	req, err := http.NewRequest("GET", fmt.Sprintf("https://%s/portfolio.php?%s", loginData.Link, encodedParams), nil)
	if err != nil {
		fmt.Println(loginData.Name+": can't get last page", err)
		return 0, errCantGetLastPage
	}
	for name, value := range cookies{
		req.AddCookie(&http.Cookie{Name: name, Value: value})
	}

	res, err := client.Do(req)
	if err != nil {
		fmt.Println(loginData.Name+": can't get last page", err)
		return 0, errCantGetLastPage
	}
	defer res.Body.Close()

	doc, err := goquery.NewDocumentFromReader(res.Body)
	if err != nil {
		fmt.Println(loginData.Name+": can't get last page", err)
		return 0, errCantGetLastPage
	}
	totalAssignmentsText := doc.Find("span > strong").Text()
	re := regexp.MustCompile("[0-9]+")
	totalAssignmentsNumber := re.FindString(totalAssignmentsText)
	if totalAssignmentsNumber == "0" {
		return 0, nil
	}
	totalAssignments, err := strconv.Atoi(totalAssignmentsNumber)
	if err != nil {
		fmt.Println(loginData.Name+": can't convert total assignments number", err)
		return 0, errCantGetLastPage
	}

	lastPage := totalAssignments / 21
	if totalAssignments % 21 != 0 {
		lastPage++
	}

	return lastPage, nil
}

func saveJSON(path string, resultData *result) { //todo
	jsonData, err := json.Marshal(*resultData)
	if err != nil {
		log.Fatalln("JSON 마샬링 중 오류 발생:", err)
	}

	err = os.WriteFile(path+".json", jsonData, 0644)
	if err != nil {
		log.Fatalln("JSON 파일 저장 중 오류 발생:", err)
	}
	fmt.Println(path+": Crawling Done!")
	fmt.Printf("%s.json 파일에 데이터가 성공적으로 저장되었습니다.\n", path)
}