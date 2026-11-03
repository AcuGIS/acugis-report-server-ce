package com.acugis.report.models;

import com.fasterxml.jackson.annotation.JsonIgnoreProperties;

import java.util.ArrayList;
import java.util.List;

@JsonIgnoreProperties(ignoreUnknown = true)
public class BrowseResult {

    private Folder folder;
    private List<Folder> folders = new ArrayList<>();
    private List<ContentItem> items = new ArrayList<>();
    private List<Breadcrumb> breadcrumbs = new ArrayList<>();

    public Folder getFolder() {
        return folder;
    }

    public void setFolder(Folder folder) {
        this.folder = folder;
    }

    public List<Folder> getFolders() {
        return folders;
    }

    public void setFolders(List<Folder> folders) {
        this.folders = folders == null ? new ArrayList<>() : folders;
    }

    public List<ContentItem> getItems() {
        return items;
    }

    public void setItems(List<ContentItem> items) {
        this.items = items == null ? new ArrayList<>() : items;
    }

    public List<Breadcrumb> getBreadcrumbs() {
        return breadcrumbs;
    }

    public void setBreadcrumbs(List<Breadcrumb> breadcrumbs) {
        this.breadcrumbs = breadcrumbs == null ? new ArrayList<>() : breadcrumbs;
    }
}
