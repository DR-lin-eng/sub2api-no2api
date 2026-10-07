package requestmodel

import (
	"bytes"
	"mime/multipart"
	"net/textproto"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestValidateJSONModelFields(t *testing.T) {
	for _, body := range []string{
		`{"model":"gpt-5.6-luna","model":"gpt-6-astra"}`,
		`{"model":"gpt-6-astra","model":"gpt-5.6-luna"}`,
		`{"model":"same","model":"same"}`,
		`{"model":null,"model":"gpt-6-astra"}`,
		`{"model":"gpt-5.6-luna","Model":"gpt-6-astra"}`,
		`{"model":"gpt-5.6-luna","\u006dodel":"gpt-6-astra"}`,
		`{"session":{"model":"gpt-5.6-luna","Model":"gpt-6-astra"}}`,
		`{"session":{"model":"gpt-5.6-luna"},"Session":{"model":"gpt-6-astra"}}`,
		"\xef\xbb\xbf{\"model\":\"cheap\",\"input\":\"a\nb\",\"model\":\"expensive\"}",
	} {
		t.Run(body, func(t *testing.T) {
			require.ErrorIs(t, ValidateJSONModelFields([]byte(body)), ErrDuplicateModelField)
		})
	}
	for _, body := range []string{
		`{"model":"gpt-6-astra","input":"hello"}`,
		`{"type":"response.create","input":"next turn"}`,
		`{"type":"session.update","session":{"model":"gpt-6-astra"}}`,
		`{"model":"gpt-6-astra","tools":[{"parameters":{"properties":{"model":{"type":"string"}}}}],"input":[{"model":"content-a"},{"model":"content-b"}]}`,
		`{"model":"gpt-6-astra","session":{"model":"gpt-6-astra"}}`,
	} {
		require.NoError(t, ValidateJSONModelFields([]byte(body)), body)
	}
}

func TestValidateModelFieldsMultipart(t *testing.T) {
	for _, fields := range [][][2]string{
		{{"model", "gpt-5.6-luna"}, {"model", "gpt-6-astra"}},
		{{"model", "gpt-5.6-luna"}, {" model ", "gpt-6-astra"}},
		{{"session", `{"model":"cheap"}`}, {"session", `{"model":"expensive"}`}},
		{{"session", `{"model":"cheap","model":"expensive"}`}},
	} {
		var body bytes.Buffer
		writer := multipart.NewWriter(&body)
		for _, field := range fields {
			require.NoError(t, writer.WriteField(field[0], field[1]))
		}
		require.NoError(t, writer.Close())
		require.ErrorIs(t, ValidateModelFields(writer.FormDataContentType(), body.Bytes()), ErrDuplicateModelField)
	}
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	require.NoError(t, writer.WriteField("model", "gpt-image-2"))
	file, err := writer.CreateFormFile("image", "input.png")
	require.NoError(t, err)
	_, err = file.Write([]byte(`{"model":"content","model":"content"}`))
	require.NoError(t, err)
	require.NoError(t, writer.Close())
	require.NoError(t, ValidateModelFields(writer.FormDataContentType(), body.Bytes()))
}

func TestValidateModelFieldsMultipartRejectsScalarModelWithBlankFilename(t *testing.T) {
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	require.NoError(t, writer.WriteField("model", "gpt-5.6-luna"))
	part, err := writer.CreatePart(textproto.MIMEHeader{
		"Content-Disposition": {`form-data; name=" model "; filename="   "`},
	})
	require.NoError(t, err)
	_, err = part.Write([]byte("gpt-6-astra"))
	require.NoError(t, err)
	require.NoError(t, writer.Close())
	require.ErrorIs(t, ValidateModelFields(writer.FormDataContentType(), body.Bytes()), ErrDuplicateModelField)
	require.ErrorIs(t, ValidateModelFields(`multipart/form-data; boundary=" `+writer.Boundary()+` "`, body.Bytes()), ErrDuplicateModelField)
}

func TestValidateModelFieldsMultipartChecksUntrimmedLiveBoundary(t *testing.T) {
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	require.NoError(t, writer.SetBoundary(" fixture"))
	require.NoError(t, writer.WriteField("session", `{"model":"gpt-5.6-luna","model":"gpt-6-astra"}`))
	require.NoError(t, writer.Close())
	require.ErrorIs(t, ValidateModelFields(writer.FormDataContentType(), body.Bytes()), ErrDuplicateModelField)

	body.Reset()
	writer = multipart.NewWriter(&body)
	require.NoError(t, writer.SetBoundary(" fixture"))
	require.NoError(t, writer.WriteField("session", `{"model":"gpt-6-astra"}`))
	require.NoError(t, writer.Close())
	require.NoError(t, ValidateModelFields(writer.FormDataContentType(), body.Bytes()))
}
