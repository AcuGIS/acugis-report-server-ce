package com.acugis.report.models;

import com.fasterxml.jackson.annotation.JsonIgnoreProperties;
import com.fasterxml.jackson.annotation.JsonProperty;

import java.util.ArrayList;
import java.util.List;

@JsonIgnoreProperties(ignoreUnknown = true)
public class FolderTreeNode {

    private Long id;
    private String name;
    private String icon;

    @JsonProperty("parent_id")
    private Long parentId;

    private List<FolderTreeNode> children = new ArrayList<>();

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

    public String getIcon() {
        return icon;
    }

    public void setIcon(String icon) {
        this.icon = icon;
    }

    public Long getParentId() {
        return parentId;
    }

    public void setParentId(Long parentId) {
        this.parentId = parentId;
    }

    public List<FolderTreeNode> getChildren() {
        return children;
    }

    public void setChildren(List<FolderTreeNode> children) {
        this.children = children == null ? new ArrayList<>() : children;
    }
}
