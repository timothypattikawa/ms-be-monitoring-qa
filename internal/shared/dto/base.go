package dto

type (
	BaseResponse struct {
		ResponseCode    string `json:"responseCode"`
		ResponseMessage string `json:"responseMessage"`
		ResponseData    any    `json:"responseData"`
	}
)
