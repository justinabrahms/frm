.PHONY: demo

demo: demo.mp4

demo.gif: demo/main.go demo/demo.tape
	cd demo && go run main.go

demo.mp4: demo.gif
	ffmpeg -y -i demo.gif -movflags faststart -pix_fmt yuv420p \
		-vf "scale=trunc(iw/2)*2:trunc(ih/2)*2" demo.mp4
