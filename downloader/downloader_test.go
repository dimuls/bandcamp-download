package downloader

import "testing"

func Test_fixAlbumJSON(t *testing.T) {
	type args struct {
		albumJSON string
	}
	tests := []struct {
		name string
		args args
		want string
	}{
		{
			name: "simple test",
			args: args{`url: "http://verbalclick.bandcamp.com" + "/album/404",`},
			want: `url: "http://verbalclick.bandcamp.com/album/404",`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := fixAlbumJSON(tt.args.albumJSON); got != tt.want {
				t.Errorf("fixAlbumJSON() = %v, want %v", got, tt.want)
			}
		})
	}
}
