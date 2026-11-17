package controller

import (
	"bytes"
	"net/url"
	"strconv"

	"github.com/gogf/gf/v2/net/ghttp"
	"github.com/xuri/excelize/v2"
)

// xlsxHeaderStyle 导出表头统一样式（加粗+浅蓝底+居中+细边框）。
func xlsxHeaderStyle(f *excelize.File) int {
	style, _ := f.NewStyle(&excelize.Style{
		Font:      &excelize.Font{Bold: true, Color: "#1F3864", Size: 11},
		Fill:      excelize.Fill{Type: "pattern", Color: []string{"#DCE6F1"}},
		Alignment: &excelize.Alignment{Horizontal: "center", Vertical: "center", WrapText: true},
		Border:    []excelize.Border{{Type: "thin", Color: "#B4C7E7"}},
	})
	return style
}

// freezeFirstRow 冻结首行（表头随滚动固定）。
func freezeFirstRow(f *excelize.File, sheet string) {
	_ = f.SetPanes(sheet, &excelize.Panes{
		Freeze: true, YSplit: 1, TopLeftCell: "A2", ActivePane: "bottomLeft",
	})
}

// writeXLSXSheet 把一张表写进 xlsx（表头加粗底色、首行冻结、列宽按表头自适应）。
func writeXLSXSheet(f *excelize.File, sheet string, headers []string, rows [][]string) {
	styleID := xlsxHeaderStyle(f)
	_ = f.SetSheetName("Sheet1", sheet)
	for i, h := range headers {
		col, err := excelize.ColumnNumberToName(i + 1)
		if err != nil {
			continue
		}
		_ = f.SetCellValue(sheet, col+"1", h)
		_ = f.SetCellStyle(sheet, col+"1", col+"1", styleID)
		w := 10.0
		for _, row := range rows {
			if i < len(row) {
				if lw := 2.2*float64(len([]rune(row[i]))) + 2; lw > w {
					w = lw
				}
			}
		}
		if w < 10 {
			w = 10
		}
		if w > 42 {
			w = 42
		}
		_ = f.SetColWidth(sheet, col, col, w)
	}
	for ri, row := range rows {
		for ci, v := range row {
			col, err := excelize.ColumnNumberToName(ci + 1)
			if err != nil {
				continue
			}
			_ = f.SetCellValue(sheet, col+strconv.Itoa(ri+2), v)
		}
	}
	_ = f.SetRowHeight(sheet, 1, 24)
	freezeFirstRow(f, sheet)
}

// xlsxFileFromSheets 生成完整 xlsx 文件内容。sheets 顺序即工作表顺序。
func xlsxFileFromSheets(sheets []xlsxSheet) ([]byte, error) {
	f := excelize.NewFile()
	for i, s := range sheets {
		name := s.Name
		if name == "" {
			name = "导出"
		}
		if i == 0 {
			f.SetSheetName("Sheet1", name)
		} else {
			created, _ := f.NewSheet(name)
			if created == 0 {
				f.SetSheetName("Sheet1", name)
			}
		}
		writeXLSXSheet(f, name, s.Headers, s.Rows)
	}
	var buf bytes.Buffer
	if err := f.Write(&buf); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// xlsxSheet 单个工作表的数据描述。
type xlsxSheet struct {
	Name    string
	Headers []string
	Rows    [][]string
}

// sendXLSX 统一 Excel(.xlsx) 导出（单工作表）。
// 口径约定（用户明确）：导出文件一律用 Excel，不再使用 CSV。
// 失败返回 error，调用方自行回 500。
func sendXLSX(r *ghttp.Request, filename, sheetName string, headers []string, rows [][]string) error {
	return sendXLSXSheets(r, filename, []xlsxSheet{{Name: sheetName, Headers: headers, Rows: rows}})
}

// sendXLSXSheets 多工作表导出（审计口径：一个维度一个工作表）。
func sendXLSXSheets(r *ghttp.Request, filename string, sheets []xlsxSheet) error {
	if len(sheets) == 0 {
		return nil
	}
	data, err := xlsxFileFromSheets(sheets)
	if err != nil {
		return err
	}
	name := filename
	if name == "" {
		name = "export.xlsx"
	}
	r.Response.Header().Set("Content-Type", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet")
	r.Response.Header().Set("Content-Disposition", "attachment; filename*=UTF-8''"+url.PathEscape(name))
	r.Response.Header().Set("Content-Length", strconv.Itoa(len(data)))
	r.Response.Write(data)
	return nil
}
