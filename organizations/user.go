package organizations

import (
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/gorilla/mux"
	"github.com/mitchellh/mapstructure"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"zuri.chat/zccore/auth"
	"zuri.chat/zccore/utils"
)

// Get a single member of an organization
func GetMember(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	member_collection, org_collection := "members", "organizations"
	orgId := mux.Vars(r)["id"]
	memId := mux.Vars(r)["mem_id"]

	memberIdhex, _ := primitive.ObjectIDFromHex(memId)

	pOrgId, err := primitive.ObjectIDFromHex(orgId)
	if err != nil {
		utils.GetError(errors.New("invalid Organisation id"), http.StatusBadRequest, w)
		return
	}

	// get organization
	orgDoc, _ := utils.GetMongoDbDoc(org_collection, bson.M{"_id": pOrgId})
	if orgDoc == nil {
		fmt.Printf("org with id %s doesn't exist!", orgId)
		utils.GetError(errors.New("org with id "+orgId+" doesn't exist!"), http.StatusBadRequest, w)
		return
	}

	orgMember, err := utils.GetMongoDbDoc(member_collection, bson.M{"org_id": orgId, "_id": memberIdhex})
	if err != nil {
		utils.GetError(err, http.StatusInternalServerError, w)
		return
	}
	var memb Member
	mapstructure.Decode(orgMember, &memb)

	utils.GetSuccess("Member retrieved successfully", orgMember, w)
}

// Get all members of an organization
func GetMembers(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	member_collection, org_collection := "members", "organizations"
	orgId := mux.Vars(r)["id"]

	pOrgId, err := primitive.ObjectIDFromHex(orgId)
	if err != nil {
		utils.GetError(errors.New("invalid id"), http.StatusBadRequest, w)
		return
	}

	// get organization
	orgDoc, _ := utils.GetMongoDbDoc(org_collection, bson.M{"_id": pOrgId})
	if orgDoc == nil {
		fmt.Printf("org with id %s doesn't exist!", orgId)
		utils.GetError(errors.New("operation failed"), http.StatusBadRequest, w)
		return
	}

	orgMembers, err := utils.GetMongoDbDocs(member_collection, bson.M{"org_id": orgId})
	if err != nil {
		utils.GetError(err, http.StatusInternalServerError, w)
		return
	}

	utils.GetSuccess("Members retrieved successfully", orgMembers, w)
}

// Add member to an organization
func CreateMember(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	org_collection, user_collection, member_collection := "organizations", "users", "members"

	sOrgId := mux.Vars(r)["id"]
	orgId, err := primitive.ObjectIDFromHex(sOrgId)
	if err != nil {
		utils.GetError(errors.New("invalid id"), http.StatusBadRequest, w)
		return
	}

	// confirm if user_id exists
	requestData := make(map[string]string)
	if err := utils.ParseJsonFromRequest(r, &requestData); err != nil {
		utils.GetError(err, http.StatusUnprocessableEntity, w)
		return
	}

	// Validating the user email
	newUserEmail, ok := requestData["user_email"]
	if !ok {
		utils.GetError(fmt.Errorf("user_email not provided"), http.StatusBadRequest, w)
		return
	}
	if !utils.IsValidEmail(newUserEmail) {
		utils.GetError(fmt.Errorf("invalid email format : %s", newUserEmail), http.StatusBadRequest, w)
		return
	}

	userDoc, _ := utils.GetMongoDbDoc(user_collection, bson.M{"email": newUserEmail})
	if userDoc == nil {
		fmt.Printf("user with email %s doesn't exist! Register User to Proceed", newUserEmail)
		utils.GetError(errors.New("user with email "+newUserEmail+" doesn't exist! Register User to Proceed"), http.StatusBadRequest, w)
		return
	}
	type GUser struct {
		ID            primitive.ObjectID
		Email         string
		Organizations []string
	}
	// convert user to struct
	var guser GUser
	mapstructure.Decode(userDoc, &guser)

	user, _ := auth.FetchUserByEmail(bson.M{"email": strings.ToLower(newUserEmail)})

	// get organization
	orgDoc, _ := utils.GetMongoDbDoc(org_collection, bson.M{"_id": orgId})
	if orgDoc == nil {
		fmt.Printf("organization with id %s doesn't exist!", orgId.String())
		utils.GetError(errors.New("organization with id "+sOrgId+" doesn't exist!"), http.StatusBadRequest, w)
		return
	}

	// check that member isn't already in the organization
	memDoc, _ := utils.GetMongoDbDocs(member_collection, bson.M{"org_id": sOrgId, "email": newUserEmail})
	if memDoc != nil {
		fmt.Printf("organization %s has member with email %s!", orgId.String(), newUserEmail)
		utils.GetError(errors.New("User is already in this organisation"), http.StatusBadRequest, w)
		return
	}

	// convert org to struct
	var org Organization
	mapstructure.Decode(orgDoc, &org)

	newMember := Member{
		Email:    user.Email,
		OrgId:    orgId,
		Role:     "member",
		Presence: "true",
		JoinedAt: time.Now(),
	}

	// conv to struct
	memStruc, err := utils.StructToMap(newMember)
	if err != nil {
		utils.GetError(err, http.StatusInternalServerError, w)
		return
	}

	// add new member to member collection
	createdMember, err := utils.CreateMongoDbDoc(member_collection, memStruc)
	if err != nil {
		utils.GetError(err, http.StatusInternalServerError, w)
		return
	}

	var uid interface{} = user.ID
	var uuid string = uid.(primitive.ObjectID).Hex()
	updateFields := make(map[string]interface{})
	user.Organizations = append(user.Organizations, sOrgId)
	updateFields["Organizations"] = user.Organizations
	fmt.Println(user.Organizations)
	_, eerr := utils.UpdateOneMongoDbDoc(user_collection, uuid, updateFields)
	if eerr != nil {
		utils.GetError(errors.New("user update failed"), http.StatusInternalServerError, w)
		return
	}
	utils.GetSuccess("Member created successfully", createdMember, w)
}

// endpoint to update a member's profile picture
func UpdateProfilePicture(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("content-Type", "application/json")
	org_collection, member_collection := "organizations", "members"

	orgId := mux.Vars(r)["id"]
	member_Id := mux.Vars(r)["mem_id"]

	requestData := make(map[string]string)
	if err := utils.ParseJsonFromRequest(r, &requestData); err != nil {
		utils.GetError(err, http.StatusUnprocessableEntity, w)
		return
	}
	image_url := requestData["image_url"]

	pMemId, err := primitive.ObjectIDFromHex(member_Id)
	if err != nil {
		utils.GetError(errors.New("invalid id"), http.StatusBadRequest, w)
		return
	}

	pOrgId, err := primitive.ObjectIDFromHex(orgId)
	if err != nil {
		utils.GetError(errors.New("invalid id"), http.StatusBadRequest, w)
		return
	}

	orgDoc, _ := utils.GetMongoDbDoc(org_collection, bson.M{"_id": pOrgId})
	if orgDoc == nil {
		fmt.Printf("org with id %s doesn't exist!", orgId)
		utils.GetError(errors.New("operation failed"), http.StatusBadRequest, w)
		return
	}

	memberDoc, _ := utils.GetMongoDbDoc(member_collection, bson.M{"_id": pMemId, "org_id": pOrgId})
	if memberDoc == nil {
		fmt.Printf("member with id %s doesn't exist!", member_Id)
		utils.GetError(errors.New("operation failed"), http.StatusBadRequest, w)
		return

	}

	result, err := utils.UpdateOneMongoDbDoc(member_collection, member_Id, bson.M{"image_url": image_url})
	if err != nil {
		utils.GetError(err, http.StatusInternalServerError, w)
		return
	}

	utils.GetSuccess("Profile picture updated", result, w)
}

// an endpoint to update a user status
func UpdateMemberStatus(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("content-type", "application/json")

	org_collection, member_collection := "organizations", "members"

	// Validate the user ID
	orgId := mux.Vars(r)["id"]
	member_Id := mux.Vars(r)["mem_id"]

	pMemId, err := primitive.ObjectIDFromHex(member_Id)
	if err != nil {
		utils.GetError(errors.New("invalid member id"), http.StatusBadRequest, w)
		return
	}

	pOrgId, err := primitive.ObjectIDFromHex(orgId)
	if err != nil {
		utils.GetError(errors.New("invalid organization id"), http.StatusBadRequest, w)
		return
	}

	requestData := make(map[string]string)
	if err := utils.ParseJsonFromRequest(r, &requestData); err != nil {
		utils.GetError(err, http.StatusUnprocessableEntity, w)
		return
	}

	member_status := requestData["status"]

	orgDoc, _ := utils.GetMongoDbDoc(org_collection, bson.M{"_id": pOrgId})
	if orgDoc == nil {
		fmt.Printf("org with id %s doesn't exist!", orgId)
		utils.GetError(errors.New("org with id %s doesn't exist"), http.StatusBadRequest, w)
		return
	}

	memberDoc, _ := utils.GetMongoDbDoc(member_collection, bson.M{"_id": pMemId, "org_id": orgId})
	if memberDoc == nil {
		fmt.Printf("member with id %s doesn't exist!", member_Id)
		utils.GetError(errors.New("member with id doesn't exist"), http.StatusBadRequest, w)
		return
	}

	result, err := utils.UpdateOneMongoDbDoc(member_collection, member_Id, bson.M{"status": member_status})
	if err != nil {
		utils.GetError(err, http.StatusInternalServerError, w)
		return
	}

	if result.ModifiedCount == 0 {
		utils.GetError(errors.New("operation failed"), http.StatusInternalServerError, w)
		return
	}

	utils.GetSuccess("status updated successfully", nil, w)
}

// Delete single member from an organization
func DeleteMember(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	member_collection, org_collection := "members", "organizations"
	orgId := mux.Vars(r)["id"]
	memberId := mux.Vars(r)["mem_id"]

	pOrgId, err := primitive.ObjectIDFromHex(orgId)
	if err != nil {
		utils.GetError(errors.New("invalid id"), http.StatusBadRequest, w)
		return
	}

	orgDoc, _ := utils.GetMongoDbDoc(org_collection, bson.M{"_id": pOrgId})
	if orgDoc == nil {
		fmt.Printf("org with id %s doesn't exist!", orgId)
		utils.GetError(errors.New("operation failed"), http.StatusBadRequest, w)
		return
	}

	query, err := primitive.ObjectIDFromHex(memberId)
	if err != nil {
		utils.GetError(errors.New("invalid id"), http.StatusBadRequest, w)
		return
	}

	member, _ := utils.GetMongoDbDoc(member_collection, bson.M{"_id": query})
	if member == nil {
		fmt.Printf("Member with ID: %s does not exists ", memberId)
		utils.GetError(errors.New("operation failed"), http.StatusBadRequest, w)
		return
	}

	delMember, _ := utils.DeleteOneMongoDoc(member_collection, memberId)
	if delMember.DeletedCount == 0 {
		utils.GetError(errors.New("operation failed"), http.StatusBadRequest, w)
		return
	}

	utils.GetSuccess("Successfully Deleted Member", nil, w)
}

// Update a member profile
func UpdateProfile(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	org_collection, member_collection := "organizations", "members"

	id := mux.Vars(r)["id"]
	memId := mux.Vars(r)["mem_id"]

	// Check if organization id is valid
	orgId, err := primitive.ObjectIDFromHex(id)
	if err != nil {
		utils.GetError(errors.New("invalid id"), http.StatusBadRequest, w)
		return
	}

	// Check if organization id is exists in the database
	orgDoc, _ := utils.GetMongoDbDoc(org_collection, bson.M{"_id": orgId})
	if orgDoc == nil {
		fmt.Printf("organization with ID: %s does not exist ", id)
		utils.GetError(errors.New("operation failed"), http.StatusBadRequest, w)
		return
	}

	// Check if member id is valid
	pMemId, err := primitive.ObjectIDFromHex(memId)
	if err != nil {
		utils.GetError(errors.New("invalid id"), http.StatusBadRequest, w)
		return
	}

	memberDoc, _ := utils.GetMongoDbDoc(member_collection, bson.M{"_id": pMemId, "org_id": id})
	if memberDoc == nil {
		fmt.Printf("member with id %s doesn't exist!", memId)
		utils.GetError(errors.New("member with id doesn't exist"), http.StatusBadRequest, w)
		return
	}

	// Get data from request
	var memberProfile Profile
	err = utils.ParseJsonFromRequest(r, &memberProfile)
	if err != nil {
		utils.GetError(err, http.StatusUnprocessableEntity, w)
		return
	}

	// convert struct to map
	mProfile, err := utils.StructToMap(memberProfile)
	if err != nil {
		utils.GetError(err, http.StatusInternalServerError, w)
		return
	}

	// Fetch and update the MemberDoc from collection
	update, err := utils.UpdateOneMongoDbDoc(member_collection, memId, mProfile)
	if err != nil {
		utils.GetError(err, http.StatusInternalServerError, w)
		return
	}

	if update.ModifiedCount == 0 {
		utils.GetError(errors.New("operation failed"), http.StatusInternalServerError, w)
		return
	}

	utils.GetSuccess("Member Profile updated succesfully", nil, w)
}

// Toggle a member's presence
func TogglePresence(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	org_collection, member_collection := "organizations", "members"
	id := mux.Vars(r)["id"]
	memId := mux.Vars(r)["mem_id"]
	// Check if organization id is valid
	orgId, err := primitive.ObjectIDFromHex(id)
	if err != nil {
		utils.GetError(errors.New("invalid id"), http.StatusBadRequest, w)
		return
	}
	// Check if organization id exists in the database
	orgDoc, _ := utils.GetMongoDbDoc(org_collection, bson.M{"_id": orgId})
	if orgDoc == nil {
		fmt.Printf("organization with ID: %s does not exist ", id)
		utils.GetError(errors.New("operation failed"), http.StatusBadRequest, w)
		return
	}
	// Check if member id is valid
	pMemId, err := primitive.ObjectIDFromHex(memId)
	if err != nil {
		utils.GetError(errors.New("invalid id"), http.StatusBadRequest, w)
		return
	}
	memberDoc, _ := utils.GetMongoDbDoc(member_collection, bson.M{"_id": pMemId, "org_id": id})
	if memberDoc == nil {
		fmt.Printf("member with id %s doesn't exist!", memId)
		utils.GetError(errors.New("member with id doesn't exist"), http.StatusBadRequest, w)
		return
	}
	org_filter := make(map[string]interface{})
	if memberDoc["presence"] == "true" {
		org_filter["presence"] = "false"
	} else {
		org_filter["presence"] = "true"
	}
	// update the presence field of the member
	update, err := utils.UpdateOneMongoDbDoc(member_collection, memId, org_filter)
	if err != nil {
		utils.GetError(err, http.StatusInternalServerError, w)
		return
	}
	if update.ModifiedCount == 0 {
		utils.GetError(errors.New("operation failed"), http.StatusInternalServerError, w)
		return
	}
	utils.GetSuccess("Member presence toggled", nil, w)
}
