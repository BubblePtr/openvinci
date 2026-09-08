package main

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

func imageRequestBody(opts *options, apimart bool) ([]byte, string, *cliError) {
	fields := map[string]any{
		"model": opts.model, "prompt": opts.prompt, "n": 1,
		"size": opts.size, "quality": opts.quality, "background": opts.background,
		"output_format": opts.format,
		"moderation":    opts.moderation,
	}
	if apimart && len(opts.images) > 0 {
		images := make([]string, 0, len(opts.images))
		for _, path := range opts.images {
			data, err := imageDataURL(path)
			if err != nil {
				return nil, "", err
			}
			images = append(images, data)
		}
		fields["image_urls"] = images
		if opts.mask != "" {
			mask, err := imageDataURL(opts.mask)
			if err != nil {
				return nil, "", err
			}
			fields["mask_url"] = mask
		}
	}
	if len(opts.images) == 0 || apimart {
		body, err := json.Marshal(fields)
		if err != nil {
			return nil, "", errorf(codeAPIError, exitAPI, "cannot encode request: %v", err)
		}
		return body, "application/json", nil
	}
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	for name, value := range fields {
		if err := writer.WriteField(name, fmt.Sprint(value)); err != nil {
			return nil, "", errorf(codeAPIError, exitAPI, "cannot encode request: %v", err)
		}
	}
	field := "image"
	if len(opts.images) > 1 {
		field = "image[]"
	}
	for _, path := range opts.images {
		if err := addImageFile(writer, field, path); err != nil {
			return nil, "", err
		}
	}
	if opts.mask != "" {
		if err := addImageFile(writer, "mask", opts.mask); err != nil {
			return nil, "", err
		}
	}
	if err := writer.Close(); err != nil {
		return nil, "", errorf(codeAPIError, exitAPI, "cannot encode request: %v", err)
	}
	return body.Bytes(), writer.FormDataContentType(), nil
}

func addImageFile(writer *multipart.Writer, field, path string) *cliError {
	data, cerr := readInputImage(path)
	if cerr != nil {
		return cerr
	}
	header := make(textproto.MIMEHeader)
	header.Set("Content-Disposition", mime.FormatMediaType("form-data", map[string]string{"name": field, "filename": filepath.Base(path)}))
	header.Set("Content-Type", http.DetectContentType(data))
	part, err := writer.CreatePart(header)
	if err == nil {
		_, err = part.Write(data)
	}
	if err != nil {
		return errorf(codeAPIError, exitAPI, "cannot encode input %s: %v", path, err)
	}
	return nil
}

// APIMart routes GPT image edits through its generations JSON protocol.
func usesAPIMartGPT(opts *options, baseURL string) bool {
	parsed, err := url.Parse(strings.TrimSpace(baseURL))
	return err == nil && strings.EqualFold(parsed.Hostname(), "api.apimart.ai") && strings.HasPrefix(opts.model, "gpt-image-")
}

func imageDataURL(path string) (string, *cliError) {
	data, err := readInputImage(path)
	if err != nil {
		return "", err
	}
	return "data:" + http.DetectContentType(data) + ";base64," + base64.StdEncoding.EncodeToString(data), nil
}

func readInputImage(path string) ([]byte, *cliError) {
	file, err := os.Open(path)
	if err != nil {
		return nil, errorf(codeReadFailed, exitIO, "cannot read %s: %v", path, err)
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return nil, errorf(codeReadFailed, exitIO, "cannot inspect %s: %v", path, err)
	}
	if !info.Mode().IsRegular() {
		return nil, errorf(codeReadFailed, exitIO, "input %s must be a regular file", path)
	}
	data, err := io.ReadAll(file)
	if err != nil {
		return nil, errorf(codeReadFailed, exitIO, "cannot read %s: %v", path, err)
	}
	return data, nil
}
