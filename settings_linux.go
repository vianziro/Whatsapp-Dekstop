//go:build linux

package main

/*
#cgo pkg-config: gtk+-3.0
#include <gtk/gtk.h>
#include <stdlib.h>

static char* chooseDownloadFolder(void) {
    GtkWidget *dialog = gtk_file_chooser_dialog_new(
        "Pilih Folder Unduhan",
        NULL,
        GTK_FILE_CHOOSER_ACTION_SELECT_FOLDER,
        "_Batal", GTK_RESPONSE_CANCEL,
        "_Pilih Folder", GTK_RESPONSE_ACCEPT,
        NULL
    );
    gtk_file_chooser_set_select_multiple(GTK_FILE_CHOOSER(dialog), FALSE);
    gtk_file_chooser_set_create_folders(GTK_FILE_CHOOSER(dialog), TRUE);

    const gchar *downloads = g_get_user_special_dir(G_USER_DIRECTORY_DOWNLOAD);
    if (downloads) {
        gtk_file_chooser_set_current_folder(GTK_FILE_CHOOSER(dialog), downloads);
    }

    gint response = gtk_dialog_run(GTK_DIALOG(dialog));
    char *selected = NULL;
    if (response == GTK_RESPONSE_ACCEPT) {
        selected = gtk_file_chooser_get_filename(GTK_FILE_CHOOSER(dialog));
    }
    gtk_widget_destroy(dialog);

    while (gtk_events_pending()) gtk_main_iteration();

    if (!selected) return NULL;
    char *dup = g_strdup(selected);
    g_free(selected);
    return dup;
}
*/
import "C"
import (
	"errors"
	"unsafe"
)

func chooseFolderDialog() (string, error) {
	path := C.chooseDownloadFolder()
	if path == nil {
		return "", errors.New("pemilihan folder dibatalkan")
	}
	defer C.free(unsafe.Pointer(path))
	return C.GoString(path), nil
}
