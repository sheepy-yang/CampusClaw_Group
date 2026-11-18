# 杨瑾文模块 · 前端输入输出冒烟结果

- 生成方式：`go test ./internal/teaching -run TestFrontendIOSummary`（与前端同路径的 HTTP 输入/输出）
- 用例数：24
- 通过：24
- 失败：0
- 原始证据：`docs/yang-evaluation/frontend-io-results.jsonl`
- 生成时间：2026-11-18T02:04:18Z

## 首页登录

| 用例 | 方法 | 路径 | HTTP | 结果 | 输入摘要 | 输出摘要 |
| --- | --- | --- | --- | --- | --- | --- |
| `login-teacher` | POST | `/api/v1/auth/login` | **200** | 通过 | username=teacher | {"token":"_DC-9PAtw0jKG4a0IJgwl1ltT1hE-sAvK_i6r2RuVyc.5vr9ZAD9XdT_v2kd9b5zOh0aWfL5VhgjfMDFNchRLlk","user":{"classId":"cl… |
| `login-student` | POST | `/api/v1/auth/login` | **200** | 通过 | username=student | {"token":"jDPLt-NkgwiVSizF_3OadIk2co61i0_mUk6FmhUzREA.hgEuVwv87dUgOcOqvUyyjSdJLukRzt09nUBVJIHpZYs","user":{"classId":"cl… |
| `login-student2` | POST | `/api/v1/auth/login` | **200** | 通过 | username=student2 | {"token":"ILZ__U0j_FsOtC7Tnd6nhMIrlUgKigFHmEu9Vv9pdPA.D2BntYMqZ6azjHQgjL-ef1tChP4IapV-9ydf-UMaSv8","user":{"classId":"cl… |

## 学生·主观题提交

| 用例 | 方法 | 路径 | HTTP | 结果 | 输入摘要 | 输出摘要 |
| --- | --- | --- | --- | --- | --- | --- |
| `list-assignments` | GET | `/api/v1/classes/class-demo/assignments` | **200** | 通过 |  | null |
| `grade-enqueue` | POST | `/api/v1/skills/multimodal-grade` | **202** | 通过 | assignmentId=asg-http-1 | submissionId=sub-beb39fec70abe001, studentId=stu-1, status=awaiting_transcription_confirm |
| `grade-poll` | GET | `/api/v1/skills/multimodal-grade/sub-beb39fec70abe001` | **200** | 通过 |  | assignmentId=asg-http-1, studentId=stu-1, status=awaiting_transcription_confirm |
| `grade-isolation` | GET | `/api/v1/skills/multimodal-grade/sub-beb39fec70abe001` | **404** | 通过 |  | code=NOT_FOUND, message=not found |

## 学生·确认转写

| 用例 | 方法 | 路径 | HTTP | 结果 | 输入摘要 | 输出摘要 |
| --- | --- | --- | --- | --- | --- | --- |
| `grade-confirm` | POST | `/api/v1/skills/multimodal-grade/confirm-transcription` | **200** | 通过 | submissionId=sub-beb39fec70abe001 | studentId=stu-1 |

## 教师·多模态批改

| 用例 | 方法 | 路径 | HTTP | 结果 | 输入摘要 | 输出摘要 |
| --- | --- | --- | --- | --- | --- | --- |
| `teacher-list` | GET | `/api/v1/skills/multimodal-grade?assignmentId=asg-http-1` | **200** | 通过 |  | null |
| `teacher-override` | POST | `/api/v1/skills/multimodal-grade/override` | **200** | 通过 | score=8.5, resultId=sub-beb39fec70abe001 | submissionId=sub-beb39fec70abe001, studentId=stu-1, authoritative=teacher, after=8.5 |

## 教师·班级讲评

| 用例 | 方法 | 路径 | HTTP | 结果 | 输入摘要 | 输出摘要 |
| --- | --- | --- | --- | --- | --- | --- |
| `review-student-forbidden` | POST | `/api/v1/skills/class-review` | **403** | 通过 | assignmentId=asg-http-1 | code=FORBIDDEN, message=forbidden |
| `review-run` | POST | `/api/v1/skills/class-review` | **200** | 通过 | assignmentId=asg-http-1 | status=generating, contentId=review-a653816d38e16eb9, anonymized=true |

## 教师·学习诊断

| 用例 | 方法 | 路径 | HTTP | 结果 | 输入摘要 | 输出摘要 |
| --- | --- | --- | --- | --- | --- | --- |
| `diagnosis-run` | POST | `/api/v1/skills/diagnosis` | **200** | 通过 | studentId=stu-1 | studentId=stu-1, status=generating, contentId=diagnosis-73dc3e8dce236a23, intercepted=false, studentVisible=false |
| `diagnosis-get` | GET | `/api/v1/contents/diagnosis-73dc3e8dce236a23` | **200** | 通过 |  | status=pending |
| `diagnosis-release` | POST | `/api/v1/skills/diagnosis/diagnosis-73dc3e8dce236a23/release` | **200** | 通过 |  | studentId=stu-1, status=published, contentId=diagnosis-73dc3e8dce236a23, studentVisible=true |

## 学生·作答回看

| 用例 | 方法 | 路径 | HTTP | 结果 | 输入摘要 | 输出摘要 |
| --- | --- | --- | --- | --- | --- | --- |
| `snapshot-manual` | POST | `/api/v1/skills/answer-replay/snapshot` | **200** | 通过 | trigger=manual | attemptId=attempt-100418-7, studentId=stu-1, trigger=manual |
| `snapshot-interval` | POST | `/api/v1/skills/answer-replay/snapshot` | **200** | 通过 | attemptId=attempt-100418-7, trigger=interval | attemptId=attempt-100418-7, studentId=stu-1, trigger=interval |
| `snapshot-keydown` | POST | `/api/v1/skills/answer-replay/snapshot` | **400** | 通过 | attemptId=attempt-100418-7, trigger=keydown | code=BAD_REQUEST, message=invalid evaluation argument: only interval|manual allowed (no keylogging) |
| `timeline-student` | GET | `/api/v1/skills/answer-replay/attempt-100418-7/timeline` | **200** | 通过 |  | attemptId=attempt-100418-7, studentId=stu-1 |
| `timeline-peer` | GET | `/api/v1/skills/answer-replay/attempt-100418-7/timeline` | **404** | 通过 |  | code=NOT_FOUND, message=not found |

## 教师·作答回放

| 用例 | 方法 | 路径 | HTTP | 结果 | 输入摘要 | 输出摘要 |
| --- | --- | --- | --- | --- | --- | --- |
| `snapshot-teacher-forbidden` | POST | `/api/v1/skills/answer-replay/snapshot` | **403** | 通过 | trigger=manual | code=FORBIDDEN, message=forbidden |
| `timeline-teacher` | GET | `/api/v1/skills/answer-replay/attempt-100418-7/timeline` | **200** | 通过 |  | attemptId=attempt-100418-7, studentId=stu-1 |
| `similarity-teacher` | GET | `/api/v1/skills/answer-replay/attempt-100418-7/similarity` | **200** | 通过 |  | attemptId=attempt-100418-7, studentId=stu-1, status=queued, verdict=<nil> |
| `similarity-student` | GET | `/api/v1/skills/answer-replay/attempt-100418-7/similarity` | **403** | 通过 |  | code=FORBIDDEN, message=forbidden |

