package com.acugis.report.models;

import com.fasterxml.jackson.annotation.JsonIgnoreProperties;
import com.fasterxml.jackson.annotation.JsonProperty;

@JsonIgnoreProperties(ignoreUnknown = true)
public class Folder {

    private Long id;
    private String name;
    private String description;
    private String icon;
    private String path;

    @JsonProperty("parent_id")
    private Long parentId;

    @JsonProperty("sort_order")
    private Integer sortOrder;

    @JsonProperty("acl_inherit")
    private Boolean aclInherit;

    @JsonProperty("child_folder_count")
    private Integer childFolderCount;

    @JsonProperty("child_item_count")
    private Integer childItemCount;

    @JsonProperty("created_at")
    private String createdAt;

    @JsonProperty("updated_at")
    private String updatedAt;

    @JsonProperty("created_by")
    private Long createdBy;

    public Long getId() {
        return id;
    }

    public void setId(Long id) {
        this.id = id;
    }

    public String getName() {
        return name;
    }

    public void setName(String name) {
        this.name = name;
    }

    public String getDescription() {
        return description;
    }

    public void setDescription(String description) {
        this.description = description;
    }

    public String getIcon() {
        return icon;
    }

    public void setIcon(String icon) {
        this.icon = icon;
    }

    public String getPath() {
        return path;
    }

    public void setPath(String path) {
        this.path = path;
    }

    public Long getParentId() {
        return parentId;
    }

    public void setParentId(Long parentId) {
        this.parentId = parentId;
    }

    public Integer getSortOrder() {
        return sortOrder;
    }

    public void setSortOrder(Integer sortOrder) {
        this.sortOrder = sortOrder;
    }

    public Boolean getAclInherit() {
        return aclInherit;
    }

    public void setAclInherit(Boolean aclInherit) {
        this.aclInherit = aclInherit;
    }

    public Integer getChildFolderCount() {
        return childFolderCount;
    }

    public void setChildFolderCount(Integer childFolderCount) {
        this.childFolderCount = childFolderCount;
    }

    public Integer getChildItemCount() {
        return childItemCount;
    }

    public void setChildItemCount(Integer childItemCount) {
        this.childItemCount = childItemCount;
    }

    public String getCreatedAt() {
        return createdAt;
    }

    public void setCreatedAt(String createdAt) {
        this.createdAt = createdAt;
    }

    public String getUpdatedAt() {
        return updatedAt;
    }

    public void setUpdatedAt(String updatedAt) {
        this.updatedAt = updatedAt;
    }

    public Long getCreatedBy() {
        return createdBy;
    }

    public void setCreatedBy(Long createdBy) {
        this.createdBy = createdBy;
    }
}
