package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os/exec"
	"strconv"
	"strings"
	"time"

	_ "github.com/go-sql-driver/mysql"
	"github.com/jmoiron/sqlx"
)

var Db *sqlx.DB

func ActionHandler(response http.ResponseWriter, request *http.Request) {
	response.Header().Add("Access-Control-Allow-Origin", "*")
	params := request.URL.Query()
	result := make(map[string]interface{})
	result["code"] = 0
	result["data"] = make([]interface{}, 0)

	switch params.Get("type") {

	case "play-video":
		videoId := params.Get("video_id")
		Db.Exec(fmt.Sprintf("insert into view_record (`video_id`, `view_time`, `ip`) values (%s, %d, '%s')", videoId, time.Now().Unix(), request.Host))

	case "delete-video":
		videoId := params.Get("video_id")
		deleted := params.Get("deleted")
		Db.Exec(fmt.Sprintf("update video set deleted = %s where id = %s", deleted, videoId))

	case "delete-tag":
		tagId := params.Get("tag_id")

		tx, _ := Db.Begin()
		tx.Exec(fmt.Sprintf("delete from video_tag where tag_id = %s", tagId))
		tx.Exec(fmt.Sprintf("delete from tag where id = %s", tagId))
		tx.Commit()

	case "create-tag":
		tagName := params.Get("tag_name")
		tagDesc := params.Get("tag_desc")
		Db.Exec(fmt.Sprintf("insert into tag (`name`, `desc`) select '%s', '%s' from dual where not exists (select id from tag where name = '%s')", tagName, tagDesc, tagName))

	case "modify-tag":
		tagName := params.Get("tag_name")
		tagId := params.Get("tag_id")
		Db.Exec(fmt.Sprintf("update tag set `name` = '%s' where id = %s", tagName, tagId))

	case "mark-star-tag":
		tagId := params.Get("tag_id")
		mark := params.Get("mark")
		if mark == "1" {
			Db.Exec(fmt.Sprintf("update tag set `mark_star` = 1 where id = %s", tagId))
		} else {
			Db.Exec(fmt.Sprintf("update tag set `mark_star` = 0 where id = %s", tagId))
		}

	case "modify-video-tag":
		videoId := params.Get("video_id")
		tagIds := strings.Split(params.Get("tag_ids"), ",")
		if len(tagIds) != 0 && tagIds[0] == "" {
			tagIds = tagIds[1:]
		}
		tx, _ := Db.Begin()
		tx.Exec(fmt.Sprintf("delete from video_tag where video_id = %s", videoId))
		for _, tagId := range tagIds {
			tx.Exec(fmt.Sprintf("insert into video_tag (`video_id`, `tag_id`) select %s, %s from dual where not exists (select id from video_tag where video_id = %s and tag_id = %s)", videoId, tagId, videoId, tagId))
		}
		_ = tx.Commit()

	case "add-high-range":
		videoId := params.Get("video_id")
		startMs := params.Get("start_ms")
		endMs := params.Get("end_ms")

		tx, _ := Db.Begin()
		tx.Exec(fmt.Sprintf("insert into video_high_range (`video_id`, `start_ms`, `end_ms`) values (%s, %s, %s)", videoId, startMs, endMs))
		tx.Commit()

	case "delete-high-range":
		id := params.Get("id")

		tx, _ := Db.Begin()
		tx.Exec(fmt.Sprintf("delete from video_high_range where id = %s", id))
		tx.Commit()

	case "parse-local-video":
		exec.Command("")

	case "export-tag-video":
		exec.Command("")
	}

	byte, _ := json.Marshal(result)
	response.Write(byte)
}

func InformationHandler(response http.ResponseWriter, request *http.Request) {

	response.Header().Add("Access-Control-Allow-Origin", "*")
	params := request.URL.Query()
	pageNumber := params.Get("page_number")
	var pageNumberInt int64
	if pageNumber == "" {
		pageNumberInt = 0
	} else {
		pageNumberInt, _ = strconv.ParseInt(pageNumber, 10, 64)
	}
	result := make(map[string]interface{})
	result["code"] = 0
	result["data"] = make([]interface{}, 0)

	switch params.Get("type") {
	case "video-list":
		keyword := strings.ToLower(params.Get("keyword"))
		tagIds := strings.Split(params.Get("tag_ids"), ",")
		queryType := params.Get("query_type")
		if len(tagIds) != 0 && tagIds[0] == "" {
			tagIds = tagIds[1:]
		}
		var sql string
		sql = `
				select id, md5, duration_ms, size_byte, width, height, title, modify_time, extension, ifnull(view_count, 0) view_count, deleted
				from video A left join (select count(0) view_count, video_id from view_record group by video_id) B on A.id = B.video_id`

		if keyword != "" {
			sql = fmt.Sprintf("%s where deleted = 0 and title COLLATE UTF8_GENERAL_CI like '%%%s%%'", sql, keyword)
		}

		if queryType == "none" {
			sql = fmt.Sprintf("%s where id not in (select distinct video_id from video_tag) order by id desc", sql)
		} else {

			if len(tagIds) == 0 {
				sql = fmt.Sprintf("%s order by id desc limit %d, %d", sql, pageNumberInt*200, 200)
			} else {
				if queryType == "and" {
					sql = fmt.Sprintf("%s where id in (select video_id from video_tag where tag_id in (%s) group by video_id having count(0) = %d) order by id desc", sql, strings.Join(tagIds, ","), len(tagIds))
				} else if queryType == "or" {
					sql = fmt.Sprintf("select id, md5, duration_ms, size_byte, width, height, title, modify_time, extension, view_count, deleted from (select distinct video_id from video_tag where tag_id in (%s)) A left join (%s) B on A.video_id = B.id order by id desc", strings.Join(tagIds, ","), sql)
				}
			}
		}

		cursor, err := Db.Query(sql)
		if err != nil {
			result["code"] = 1
			break
		}

		for cursor.Next() {
			var md5, title, extension string
			var id, duration_ms, size_byte, width, height, modify_time, view_count, deleted int64
			err := cursor.Scan(&id, &md5, &duration_ms, &size_byte, &width, &height, &title, &modify_time, &extension, &view_count, &deleted)
			if err != nil || deleted == 1 {
				continue
			}
			row := make(map[string]interface{})
			row["id"] = id
			row["md5"] = md5
			row["duration_ms"] = duration_ms
			row["size_byte"] = size_byte
			row["width"] = width
			row["height"] = height
			row["title"] = title
			row["modify_time"] = modify_time
			row["view_count"] = view_count
			row["deleted"] = deleted;
			row["jpg"] = fmt.Sprintf("E:/VBrowser/Thumbnail-IMG/%s.jpg", md5)
			row["gif"] = fmt.Sprintf("E:/VBrowser/Thumbnail-GIF/%s.gif", md5)
			row["src"] = fmt.Sprintf("E:/VBrowser/Video/%s.%s", md5, extension)
			result["data"] = append(result["data"].([]interface{}), row)
		}

	case "deleted-video-list":
		var sql string
		sql = `select id, md5, duration_ms, size_byte, width, height, title, modify_time, extension from video where deleted = 1 order by id desc`

		cursor, err := Db.Query(sql)
		if err != nil {
			result["code"] = 1
			break
		}

		for cursor.Next() {
			var md5, title, extension string
			var id, duration_ms, size_byte, width, height, modify_time int64
			err := cursor.Scan(&id, &md5, &duration_ms, &size_byte, &width, &height, &title, &modify_time, &extension)
			if err != nil {
				continue
			}
			row := make(map[string]interface{})
			row["id"] = id
			row["md5"] = md5
			row["duration_ms"] = duration_ms
			row["size_byte"] = size_byte
			row["width"] = width
			row["height"] = height
			row["title"] = title
			row["modify_time"] = modify_time
			row["deleted"] = 1
			row["jpg"] = fmt.Sprintf("E:/VBrowser/Thumbnail-IMG/%s.jpg", md5)
			row["gif"] = fmt.Sprintf("E:/VBrowser/Thumbnail-GIF/%s.gif", md5)
			row["src"] = fmt.Sprintf("E:/VBrowser/Video/%s.%s", md5, extension)
			result["data"] = append(result["data"].([]interface{}), row)
		}

	case "tag-list":
		cursor, err := Db.Query("select `id`, `name`, `desc`, ifnull(`count`, 0) `count`, `mark_star` from tag A left join (select count(0) count, tag_id from video_tag group by tag_id) B on A.id = B.tag_id order by `name` asc")
		if err != nil {
			log.Fatal(err)
		}
		for cursor.Next() {
			var name, desc string
			var id, count, markStar int64
			err := cursor.Scan(&id, &name, &desc, &count, &markStar)
			if err != nil {
				continue
			}
			row := make(map[string]interface{})
			row["id"] = id
			row["name"] = name
			row["desc"] = desc
			row["count"] = count
			row["mark_star"] = markStar
			result["data"] = append(result["data"].([]interface{}), row)
		}

	case "video":
		videoId := params.Get("video_id")
		if videoId == "" {
			result["code"] = 1
		} else {
			sql := `
					select id, md5, duration_ms, size_byte, width, height, title, modify_time, extension, view_count, tag_names, tag_ids, deleted  
					from 
						(
							select A.*, ifnull(view_count, 0) view_count 
							from 
								video A 
								left join 
								(select count(0) view_count, video_id from view_record group by video_id) B 
								on A.id = B.video_id
								where A.id = %s
						) A
						left join 
						(
							select group_concat(name) tag_names, group_concat(id) tag_ids, video_id 
							from 
							(select tag_id, video_id from video_tag where video_id = %s) A
							left join 
							tag B 
							on A.tag_id = B.id group by video_id
						) B 
						on A.id = B.video_id
					`
			cursor, err := Db.Query(fmt.Sprintf(sql, videoId, videoId))
			if err != nil || !cursor.Next() {
				result["code"] = 1
			} else {
				var md5, title, extension, tag_names, tag_ids string
				var id, duration_ms, size_byte, width, height, modify_time, view_count, deleted int64
				err := cursor.Scan(&id, &md5, &duration_ms, &size_byte, &width, &height, &title, &modify_time, &extension, &view_count, &tag_names, &tag_ids, &deleted)
				if err != nil {
					break
				}

				row := make(map[string]interface{})
				row["id"] = id
				row["md5"] = md5
				row["duration_ms"] = duration_ms
				row["size_byte"] = size_byte
				row["width"] = width
				row["height"] = height
				row["title"] = title
				row["modify_time"] = modify_time
				row["view_count"] = view_count
				row["deleted"] = deleted
				row["jpg"] = fmt.Sprintf("/Volumes/ProtectedFiles/VBrowser/Thumbnail-IMG/%s.jpg", md5)
				row["gif"] = fmt.Sprintf("/Volumes/ProtectedFiles/VBrowser/Thumbnail-GIF/%s.gif", md5)
				row["src"] = fmt.Sprintf("/Volumes/ProtectedFiles/VBrowser/Video/%s.%s", md5, extension)
				row["tag_names"] = strings.Split(tag_names, ",")
				row["tag_ids"] = strings.Split(tag_ids, ",")
				var highRangeMaps = make([]map[string]int, 0)

				highRangeCursor, highRangeErr := Db.Query(fmt.Sprintf("select id, start_ms, end_ms from video_high_range where video_id = %s", videoId))
				if highRangeErr == nil {
					for highRangeCursor.Next() {
						var id, startMs, endMs int
						e := highRangeCursor.Scan(&id, &startMs, &endMs)
						if e == nil {
							var highRangeMap = make(map[string]int)
							highRangeMap["id"] = id
							highRangeMap["start_ms"] = startMs
							highRangeMap["end_ms"] = endMs
							highRangeMap["video_id"], e = strconv.Atoi(videoId)
							highRangeMaps = append(highRangeMaps, highRangeMap)
						}
					}
				}
				row["high_ranges"] = highRangeMaps
				result["data"] = row
			}
		}

	case "image-album-list":
		row := make(map[string]interface{})

		var albums = make([]map[string]interface{}, 0)
		path := "E:/VBrowser/Picture-Thumbnail/"
		fs, _ := ioutil.ReadDir(path)
		for _, file := range fs {
			if file.IsDir() && !strings.HasPrefix(file.Name(), ".") {

				album := make(map[string]interface{})
				album["name"] = file.Name()
				album["path"] = path + "/" + file.Name()

				images := make([]string, 0)
				sfs, _ := ioutil.ReadDir(album["path"].(string))

				var count = 0
				for _, sfile := range sfs {

					if !sfile.IsDir() && !strings.HasPrefix(sfile.Name(), ".") {

						count++
						if count <= 6 {
							images = append(images, album["path"].(string)+"/"+sfile.Name())
						}
					}
				}
				// TODO

				album["images"] = images
				album["count"] = count

				albums = append(albums, album)
			}
		}

		row["albums"] = albums
		result["data"] = row

	case "image-album-detail":
		albumName := params.Get("album_name")
		row := make(map[string]interface{})

		var images = make([]string, 0)
		// path := "/Users/holobor/Downloads"
		path := "E:/VBrowser/Picture-Src/"
		fs, _ := ioutil.ReadDir(path + "/" + albumName)
		for _, file := range fs {
			if !file.IsDir() && !strings.HasPrefix(file.Name(), ".") {
				images = append(images, path+"/"+albumName+"/"+file.Name())
			}
		}
		row["name"] = albumName
		row["images"] = images
		result["data"] = row

	default:
		result["code"] = 1
	}

	byte, _ := json.Marshal(result)
	response.Write(byte)
}

func main() {

	db, err := sqlx.Open("mysql", "root:jskk19931013@tcp(localhost:3306)/vbrowser?charset=utf8")
	if err != nil {
		log.Fatal(err)
		return
	}
	Db = db
	http.HandleFunc("/action", ActionHandler)
	http.HandleFunc("/information", InformationHandler)
	err = http.ListenAndServe(":8880", nil)
	if err != nil {
		log.Fatal(err)
	}
}
