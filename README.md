
# 리로스쿨 과제 크롤러
Project.Echos라는 학생을 위한 리로스쿨 과제 관리 사이트에 사용되는 리로스쿨 크롤러 소스코드입니다.

확장성을 고려하여 여러 학교의 과제를 크롤링할 수 있도록 코드가 구성되어있습니다.

Go routine을 최대한으로 활용하여 빠른 속도를 보장합니다.

---
### 사용방법
1. `Data.json`을 [Data.json.example](Data.json.example)을 참고하여 작성합니다.
2. `main.go` 파일을 실행합니다.
3. `학교명.json` 파일이 `main.go` 파일과 동일한 경로에 생성됩니다.

### 결과 JSON 파일 예시
```json
{
  "success": true,
  "lastUpdate": 1725516010,
  "firstGrade": [
    {
        "number": "1-63",
        "teacher": "OOO",
        "subject": "기타",
        "title": "피지컬컴퓨팅 사례",
        "time": 1725786000,
        "isEnded": false
    }
  ],
  "secondGrade": [  ],
  "thirdGrade": [  ]
}
```
#### 각 속성의 의미
- success: 크롤링 성공 여부
- lastUpdate: 크롤링 시각(Unixtime)
- firstGrade, secondGrade, thirdGrade: 학년 정보
    - number: 수행평가 아이디 ("과제 종류-과제 순번")
        - 1-XX: 수행평가
        - 2-XX: 경시대회
        - 3-XX: 포트폴리오
    - teacher: 담당 선생님
    - subject: 과목(수행평가의 경우)
    - title: 과제 이름
    - time: 제출 마감일
    - isEnded: 마감 여부
